package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
)

type ProductHandler struct {
	Supabase        *supabase.Client
	StorageBucket   string
	MaxUploadSizeMB int64
}

func NewProductHandler(
	client *supabase.Client,
	storageBucket string,
	maxUploadSizeMB int64,
) *ProductHandler {
	return &ProductHandler{
		Supabase:        client,
		StorageBucket:   storageBucket,
		MaxUploadSizeMB: maxUploadSizeMB,
	}
}

func (h *ProductHandler) CreateProduct(c *gin.Context) {
	if h.Supabase == nil || h.Supabase.Storage == nil ||
		h.StorageBucket == "" || h.MaxUploadSizeMB <= 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Product upload settings are not configured.",
		})
		return
	}

	maxUploadBytes := h.MaxUploadSizeMB * 1024 * 1024

	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxUploadBytes+(1<<20),
	)

	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "The uploaded request is too large.",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid multipart form data.",
		})
		return
	}

	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	// Read product fields.
	productName := strings.TrimSpace(c.PostForm("product_name"))
	productDetails := strings.TrimSpace(c.PostForm("product_details"))

	// Validate product name.
	if productName == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Product name cannot be empty.",
		})
		return
	}

	if utf8.RuneCountInString(productName) > 120 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Product name must be 120 characters or fewer.",
		})
		return
	}

	// Validate unit price.
	priceString := strings.TrimSpace(c.PostForm("unit_price"))

	unitPrice, err := decimal.NewFromString(priceString)
	if err != nil || !unitPrice.IsPositive() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Unit price must be greater than zero.",
		})
		return
	}

	// Parse active. Defaults to true when omitted.
	active := true

	if activeString, exists := c.GetPostForm("active"); exists {
		active, err = strconv.ParseBool(activeString)

		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "Active must be a valid boolean.",
			})
			return
		}
	}

	// Locate optional image.
	var imageHeader *multipart.FileHeader

	imageFiles := c.Request.MultipartForm.File["image"]

	if len(imageFiles) > 1 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Only one product image may be uploaded.",
		})
		return
	}

	if len(imageFiles) == 1 {
		imageHeader = imageFiles[0]
	}

	// If an image is uploaded but the database insert fails,
	// remove the image from Storage.
	var storagePath string
	productSaved := false

	defer func() {
		if storagePath != "" && !productSaved {
			_, err := h.Supabase.Storage.RemoveFile(
				h.StorageBucket,
				[]string{storagePath},
			)

			if err != nil {
				log.Printf(
					"Failed to clean up product image %q: %v",
					storagePath,
					err,
				)
			}
		}
	}()

	var publicImageURL interface{}

	// Upload optional product image.
	if imageHeader != nil {
		if imageHeader.Size > maxUploadBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf(
					"Image must be %d MB or smaller.",
					h.MaxUploadSizeMB,
				),
			})
			return
		}

		file, err := imageHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Could not read the uploaded image.",
			})
			return
		}
		defer file.Close()

		content, err := io.ReadAll(
			io.LimitReader(file, maxUploadBytes+1),
		)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Could not read the uploaded image.",
			})
			return
		}

		if len(content) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "The uploaded image is empty.",
			})
			return
		}

		if int64(len(content)) > maxUploadBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf(
					"Image must be %d MB or smaller.",
					h.MaxUploadSizeMB,
				),
			})
			return
		}

		// Reuse detectImageType from order_handler.go.
		// It checks the actual image signature.
		contentType, extension, validImage := detectImageType(content)

		if !validImage {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"error": "Only JPEG, PNG, and WebP images are supported.",
			})
			return
		}

		storagePath = uuid.New().String() + "." + extension

		upsert := false

		_, err = h.Supabase.Storage.UploadFile(
			h.StorageBucket,
			storagePath,
			bytes.NewReader(content),
			storage_go.FileOptions{
				ContentType: &contentType,
				Upsert:      &upsert,
			},
		)

		if err != nil {
			log.Printf("Product image upload failed: %v", err)
			storagePath = ""

			c.JSON(http.StatusBadGateway, gin.H{
				"error": "Could not upload the product image.",
			})
			return
		}

		// GetPublicUrl returns a SignedUrlResponse directly.
		// Its SignedURL field contains the public URL.
		publicImageURL = h.Supabase.Storage.
			GetPublicUrl(h.StorageBucket, storagePath).
			SignedURL
	}

	// Build database record.
	productData := map[string]interface{}{
		"product_name":       productName,
		"unit_price":         unitPrice.String(),
		"product_details":    productDetails,
		"product_image_path": publicImageURL,
		"active":             active,
	}

	// Insert product and request the inserted row back.
	result, _, err := h.Supabase.
		From("products").
		Insert(productData, false, "", "representation", "").
		Execute()

	if err != nil {
		log.Printf("Product insertion failed: %v", err)

		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Could not create the product. Check your Supabase configuration.",
		})
		return
	}

	var createdProducts []map[string]interface{}

	if err := json.Unmarshal(result, &createdProducts); err != nil ||
		len(createdProducts) == 0 {
		log.Printf("Could not decode created product: %v", err)

		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Product was inserted, but its response could not be retrieved.",
		})
		return
	}

	// Keep the uploaded image because the database now references it.
	productSaved = true

	c.JSON(http.StatusCreated, createdProducts[0])
}
