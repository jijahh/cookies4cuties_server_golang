package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jijahh/cookies4cuties_server_golang/internal/services"
	"github.com/shopspring/decimal"
	"github.com/supabase-community/postgrest-go"
	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
)

type OrderHandler struct {
	Supabase        *supabase.Client
	StorageBucket   string
	MaxUploadSizeMB int64
	EmailService    *services.EmailService
}

func NewOrderHandler(
	client *supabase.Client,
	bucket string,
	maxUploadSizeMB int64,
	emailService *services.EmailService,
) *OrderHandler {
	return &OrderHandler{
		Supabase:        client,
		StorageBucket:   bucket,
		MaxUploadSizeMB: maxUploadSizeMB,
		EmailService:    emailService,
	}
}

type OrderFilters struct {
	OrderStatus     *string `form:"order_status"`
	CustomerName    *string `form:"customer_name"`
	PaymentMethod   *string `form:"payment_method"`
	PaymentVerified *bool   `form:"payment_verified"`
	Email           *string `form:"email"`
	PickupLocation  *string `form:"pickup_location"`

	CreatedFrom *string `form:"created_from"`
	CreatedTo   *string `form:"created_to"`

	MinTotal *string `form:"min_total"`
	MaxTotal *string `form:"max_total"`

	SortBy    string `form:"sort_by"`
	SortOrder string `form:"sort_order"`

	Limit  *int `form:"limit"`
	Offset *int `form:"offset"`
}

type OrderItemInput struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type OrderProduct struct {
	ProductID   int             `json:"product_id"`
	ProductName string          `json:"product_name"`
	UnitPrice   decimal.Decimal `json:"unit_price"`
}

type OrderReceiptLine struct {
	Quantity int
	Subtotal decimal.Decimal
}

var allowedPickupLocations = map[string]struct{}{
	"pampanga":  {},
	"katipunan": {},
	"rockwell":  {},
}
var allowedPaymentMethods = map[string]struct{}{
	"cash_on_pickup":   {},
	"cash_on_delivery": {},
	"online":           {},
}

func failOrder(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": message,
	})
}

func containsString(values map[string]struct{}, value string) bool {
	_, exists := values[value]
	return exists
}

func detectImageType(content []byte) (string, string, bool) {
	if len(content) >= 3 &&
		bytes.Equal(content[:3], []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg", "jpg", true
	}

	if len(content) >= 8 &&
		bytes.Equal(content[:8], []byte{
			0x89, 0x50, 0x4E, 0x47,
			0x0D, 0x0A, 0x1A, 0x0A,
		}) {
		return "image/png", "png", true
	}

	if len(content) >= 12 &&
		string(content[:4]) == "RIFF" &&
		string(content[8:12]) == "WEBP" {
		return "image/webp", "webp", true
	}

	return "", "", false
}

func (h *OrderHandler) CreateOrder(c *gin.Context) {
	if h.Supabase == nil || h.Supabase.Storage == nil {
		failOrder(c, http.StatusServiceUnavailable, "Supabase is not configured.")
		return
	}

	if h.StorageBucket == "" || h.MaxUploadSizeMB <= 0 {
		failOrder(c, http.StatusServiceUnavailable, "Order upload settings are not configured.")
		return
	}

	maxUploadBytes := h.MaxUploadSizeMB * 1024 * 1024

	// Limit the entire multipart request to the upload limit plus
	// a little room for the other form fields and multipart boundaries.
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxUploadBytes+(1<<20),
	)

	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(err, &maxBytesError) {
			failOrder(c, http.StatusRequestEntityTooLarge, "The uploaded request is too large.")
			return
		}

		failOrder(c, http.StatusBadRequest, "Invalid multipart form data.")
		return
	}

	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	// If an image is uploaded but a later step fails, remove it.
	var storagePath string
	orderSaved := false

	defer func() {
		if storagePath != "" && !orderSaved {
			_, err := h.Supabase.Storage.RemoveFile(
				h.StorageBucket,
				[]string{storagePath},
			)

			if err != nil {
				log.Printf("Failed to clean up uploaded proof %q: %v", storagePath, err)
			}
		}
	}()

	// Read form fields.
	customerName := strings.TrimSpace(c.PostForm("customer_name"))
	email := strings.TrimSpace(c.PostForm("email"))
	phoneNumber := strings.TrimSpace(c.PostForm("phone_number"))
	itemsJSON := c.PostForm("items")
	submittedTotal := c.PostForm("total_price")
	pickupLocation := strings.TrimSpace(c.PostForm("pickup_location"))
	paymentMethod := strings.TrimSpace(c.PostForm("payment_method"))

	// Validate basic customer information.
	atIndex := strings.LastIndex(email, "@")

	if atIndex <= 0 ||
		strings.Count(email, "@") != 1 ||
		!strings.Contains(email[atIndex+1:], ".") {
		failOrder(c, http.StatusUnprocessableEntity, "Invalid email address.")
		return
	}

	if customerName == "" {
		failOrder(c, http.StatusUnprocessableEntity, "Customer name is required.")
		return
	}

	if phoneNumber == "" {
		failOrder(c, http.StatusUnprocessableEntity, "Phone number is required.")
		return
	}

	if !containsString(allowedPickupLocations, pickupLocation) {
		failOrder(c, http.StatusUnprocessableEntity, "Invalid pickup location.")
		return
	}

	if !containsString(allowedPaymentMethods, paymentMethod) {
		failOrder(c, http.StatusUnprocessableEntity, "Invalid payment method.")
		return
	}

	// Keep total_price as a required form field, like the FastAPI version.
	// However, we will calculate the actual total using database prices.
	submittedPrice, err := decimal.NewFromString(submittedTotal)
	if err != nil || !submittedPrice.IsPositive() {
		failOrder(c, http.StatusUnprocessableEntity, "Total price must be greater than zero.")
		return
	}

	// Parse and validate order items.
	var parsedItems []OrderItemInput

	if err := json.Unmarshal([]byte(itemsJSON), &parsedItems); err != nil {
		failOrder(c, http.StatusUnprocessableEntity, "Invalid order items: "+err.Error())
		return
	}

	if len(parsedItems) == 0 {
		failOrder(c, http.StatusUnprocessableEntity, "Items must be a non-empty JSON array.")
		return
	}

	if len(parsedItems) > 20 {
		failOrder(c, http.StatusUnprocessableEntity, "Too many distinct items.")
		return
	}

	for _, item := range parsedItems {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			failOrder(
				c,
				http.StatusUnprocessableEntity,
				"Product IDs and quantities must be positive.",
			)
			return
		}
	}

	// Locate the optional uploaded image.
	var proofFile *multipart.FileHeader

	files := c.Request.MultipartForm.File["proof_of_payment"]

	if len(files) > 1 {
		failOrder(c, http.StatusUnprocessableEntity, "Only one proof image may be uploaded.")
		return
	}

	if len(files) == 1 {
		proofFile = files[0]
	}

	// Online payment requires proof of payment.
	if paymentMethod == "online" && proofFile == nil {
		failOrder(
			c,
			http.StatusUnprocessableEntity,
			"Please upload proof of payment for online orders.",
		)
		return
	}

	// Cash payments must not include proof of payment.
	if paymentMethod != "online" && proofFile != nil {
		failOrder(
			c,
			http.StatusUnprocessableEntity,
			"Proof of payment is only accepted for online payments.",
		)
		return
	}

	// Fetch products and calculate the actual total using database prices.
	// Do this before uploading the image so invalid product IDs don't
	// result in unnecessary Storage uploads.
	productIDs := make([]string, 0, len(parsedItems))
	seenProductIDs := make(map[int]struct{})

	for _, item := range parsedItems {
		if _, exists := seenProductIDs[item.ProductID]; exists {
			continue
		}
		seenProductIDs[item.ProductID] = struct{}{}
		productIDs = append(productIDs, strconv.Itoa(item.ProductID))
	}

	productData, _, err := h.Supabase.
		From("products").
		Select("product_id, product_name, unit_price", "", false).
		In("product_id", productIDs).
		Execute()

	if err != nil {
		log.Printf("Failed to fetch products: %v", err)
		failOrder(c, http.StatusBadGateway, "Could not retrieve product information.")
		return
	}

	var products []OrderProduct

	if err := json.Unmarshal(productData, &products); err != nil {
		log.Printf("Failed to decode products: %v", err)
		failOrder(c, http.StatusBadGateway, "Could not retrieve product information.")
		return
	}

	productsByID := make(map[int]OrderProduct, len(products))

	for _, product := range products {
		productsByID[product.ProductID] = product
	}

	// Make sure every requested product exists.
	for _, item := range parsedItems {
		if _, exists := productsByID[item.ProductID]; !exists {
			failOrder(
				c,
				http.StatusUnprocessableEntity,
				fmt.Sprintf("Product with ID %d does not exist.", item.ProductID),
			)
			return
		}
	}

	validatedTotalPrice := decimal.Zero
	productPriceReceipt := make(map[string]OrderReceiptLine)

	for _, item := range parsedItems {
		product := productsByID[item.ProductID]

		quantity := decimal.NewFromInt(int64(item.Quantity))
		subtotal := product.UnitPrice.Mul(quantity)

		validatedTotalPrice = validatedTotalPrice.Add(subtotal)

		productPriceReceipt[product.ProductName] = OrderReceiptLine{
			Quantity: item.Quantity,
			Subtotal: subtotal,
		}
	}

	// Upload payment proof using the Storage client already inside
	// the existing Supabase client.
	var publicImageURL interface{}

	if proofFile != nil {
		if proofFile.Size > maxUploadBytes {
			failOrder(
				c,
				http.StatusRequestEntityTooLarge,
				fmt.Sprintf("Image must be %d MB or smaller.", h.MaxUploadSizeMB),
			)
			return
		}

		file, err := proofFile.Open()
		if err != nil {
			failOrder(c, http.StatusBadRequest, "Could not read the uploaded image.")
			return
		}
		defer file.Close()

		content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
		if err != nil {
			failOrder(c, http.StatusBadRequest, "Could not read the uploaded image.")
			return
		}

		if len(content) == 0 {
			failOrder(c, http.StatusBadRequest, "The uploaded image is empty.")
			return
		}

		if int64(len(content)) > maxUploadBytes {
			failOrder(
				c,
				http.StatusRequestEntityTooLarge,
				fmt.Sprintf("Image must be %d MB or smaller.", h.MaxUploadSizeMB),
			)
			return
		}

		contentType, extension, validImage := detectImageType(content)

		if !validImage {
			failOrder(
				c,
				http.StatusUnsupportedMediaType,
				"Only JPEG, PNG, and WebP images are supported.",
			)
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
			log.Printf("Failed to upload payment proof: %v", err)
			// Clear the path because the upload did not report success.
			storagePath = ""
			failOrder(c, http.StatusBadGateway, "Could not upload proof of payment.")
			return
		}

		// GetPublicUrl returns a URL for a public bucket.
		publicImageURL = h.Supabase.Storage.
			GetPublicUrl(h.StorageBucket, storagePath).
			SignedURL
	}

	// Build the order record.
	orderData := map[string]interface{}{
		"email":            email,
		"phone":            phoneNumber,
		"items":            parsedItems,
		"total_price":      validatedTotalPrice.StringFixed(2),
		"pickup_location":  pickupLocation,
		"payment_method":   paymentMethod,
		"proof_url":        publicImageURL,
		"order_status":     "pending",
		"payment_verified": false,
		"customer_name":    customerName,
	}

	// Insert the order and request the inserted row back.
	result, _, err := h.Supabase.
		From("orders").
		Insert(orderData, false, "", "representation", "").
		Execute()

	if err != nil {
		log.Printf("Failed to insert order: %v", err)
		failOrder(c, http.StatusBadGateway, "Could not create the order.")
		return
	}

	// The insert succeeded. Keep the uploaded proof because the order
	// now references it.
	orderSaved = true

	var createdOrders []map[string]interface{}

	if err := json.Unmarshal(result, &createdOrders); err != nil || len(createdOrders) == 0 {
		log.Printf("Could not decode inserted order: %v", err)
		failOrder(c, http.StatusBadGateway, "The order was created, but its response could not be retrieved.")
		return
	}

	receiptItems := make([]services.ReceiptItem, 0, len(productPriceReceipt))

	for productName, item := range productPriceReceipt {
		receiptItems = append(receiptItems, services.ReceiptItem{
			Name:     productName,
			Quantity: item.Quantity,
			Subtotal: item.Subtotal,
		})
	}

	if err := h.EmailService.SendOrderEmail(services.OrderEmailData{
		RecipientEmail: email,
		OrderStatus:    "pending",
		PickupLocation: pickupLocation,
		PaymentMethod:  strings.ReplaceAll(paymentMethod, "_", " "),
		Items:          receiptItems,
		TotalPrice:     validatedTotalPrice,
	}); err != nil {
		log.Printf("Failed to send order confirmation email: %v", err)
	}

	c.JSON(http.StatusCreated, createdOrders[0])
}

func (h *OrderHandler) UpdateOrder(c *gin.Context) {
	// Parse order ID from URL.
	orderID, err := strconv.Atoi(c.Param("order_id"))

	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid order ID.",
		})
		return
	}

	// Read optional form fields.
	updateData := make(map[string]interface{})

	if orderStatus, exists := c.GetPostForm("order_status"); exists {
		orderStatus = strings.TrimSpace(orderStatus)

		if orderStatus != "" {
			updateData["order_status"] = orderStatus
		}
	}

	if paymentVerified, exists := c.GetPostForm("payment_verified"); exists {
		verified, err := strconv.ParseBool(paymentVerified)

		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "Payment verified must be a valid boolean.",
			})
			return
		}

		updateData["payment_verified"] = verified
	}

	// Make sure at least one field was provided.
	if len(updateData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No fields provided for update.",
		})
		return
	}

	// Update order in Supabase.
	result, _, err := h.Supabase.
		From("orders").
		Update(updateData, "representation", "").
		Eq("id", strconv.Itoa(orderID)).
		Execute()

	if err != nil {
		log.Printf("Order update failed: %v", err)

		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Could not update the order. Check your Supabase configuration.",
		})
		return
	}

	var updatedOrders []map[string]interface{}

	if err := json.Unmarshal(result, &updatedOrders); err != nil {
		log.Printf("Could not decode updated order: %v", err)

		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Could not retrieve the updated order.",
		})
		return
	}

	if len(updatedOrders) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Order not found.",
		})
		return
	}

	c.JSON(http.StatusCreated, updatedOrders)
}
func (h *OrderHandler) GetOrders(c *gin.Context) {
	var filters OrderFilters

	// Default values
	filters.SortBy = "created_at"
	filters.SortOrder = "desc"

	limit := 50
	offset := 0

	// Bind query parameters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
		})
		return
	}

	if filters.Limit != nil {
		limit = *filters.Limit
	}

	if filters.Offset != nil {
		offset = *filters.Offset
	}

	// Validate pagination
	if limit < 1 || limit > 200 || offset < 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Invalid limit or offset.",
		})
		return
	}

	// Validate sorting
	allowedSortColumns := map[string]bool{
		"created_at":      true,
		"total_price":     true,
		"order_status":    true,
		"email":           true,
		"pickup_location": true,
		"payment_method":  true,
	}

	if !allowedSortColumns[filters.SortBy] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Invalid sort_by value.",
		})
		return
	}

	if filters.SortOrder != "asc" && filters.SortOrder != "desc" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "sort_order must be asc or desc.",
		})
		return
	}

	// Validate date ranges
	var createdFrom, createdTo *time.Time

	if filters.CreatedFrom != nil {
		parsed, err := time.Parse(time.RFC3339, *filters.CreatedFrom)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "Invalid created_from. Use RFC3339 format.",
			})
			return
		}
		createdFrom = &parsed
	}

	if filters.CreatedTo != nil {
		parsed, err := time.Parse(time.RFC3339, *filters.CreatedTo)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "Invalid created_to. Use RFC3339 format.",
			})
			return
		}
		createdTo = &parsed
	}

	if createdFrom != nil && createdTo != nil &&
		createdFrom.After(*createdTo) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "created_from must be earlier than created_to.",
		})
		return
	}

	// Validate total price ranges
	minTotal, err := parseNonNegativeDecimal(filters.MinTotal)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Invalid min_total. Must be a non-negative number.",
		})
		return
	}

	maxTotal, err := parseNonNegativeDecimal(filters.MaxTotal)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Invalid max_total. Must be a non-negative number.",
		})
		return
	}

	if minTotal != nil && maxTotal != nil &&
		minTotal.Cmp(maxTotal) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "min_total cannot exceed max_total.",
		})
		return
	}

	// Start Supabase query
	query := h.Supabase.
		From("orders").
		Select("*", "", false)

	// Optional filters
	if filters.OrderStatus != nil && *filters.OrderStatus != "" {
		query = query.Eq("order_status", *filters.OrderStatus)
	}

	if filters.PaymentMethod != nil && *filters.PaymentMethod != "" {
		query = query.Eq("payment_method", *filters.PaymentMethod)
	}

	if filters.PaymentVerified != nil {
		query = query.Eq(
			"payment_verified",
			strconv.FormatBool(*filters.PaymentVerified),
		)
	}

	if filters.Email != nil && strings.TrimSpace(*filters.Email) != "" {
		query = query.Ilike(
			"email",
			"%"+strings.TrimSpace(*filters.Email)+"%",
		)
	}

	if filters.CustomerName != nil &&
		strings.TrimSpace(*filters.CustomerName) != "" {
		query = query.Ilike(
			"customer_name",
			"%"+strings.TrimSpace(*filters.CustomerName)+"%",
		)
	}

	if filters.PickupLocation != nil && *filters.PickupLocation != "" {
		query = query.Eq("pickup_location", *filters.PickupLocation)
	}

	if createdFrom != nil {
		query = query.Gte(
			"created_at",
			createdFrom.Format(time.RFC3339),
		)
	}

	if createdTo != nil {
		query = query.Lte(
			"created_at",
			createdTo.Format(time.RFC3339),
		)
	}

	if filters.MinTotal != nil {
		query = query.Gte("total_price", strings.TrimSpace(*filters.MinTotal))
	}

	if filters.MaxTotal != nil {
		query = query.Lte("total_price", strings.TrimSpace(*filters.MaxTotal))
	}

	// Sorting
	query = query.Order(
		filters.SortBy,
		&postgrest.OrderOpts{
			Ascending: filters.SortOrder == "asc",
		},
	)

	// Pagination
	query = query.Range(offset, offset+limit-1, "")

	// Execute query
	data, _, err := query.Execute()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve orders.",
		})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

// Parses a non-negative decimal without converting it to float64.
func parseNonNegativeDecimal(value *string) (*big.Rat, error) {
	if value == nil {
		return nil, nil
	}

	parsed, ok := new(big.Rat).SetString(strings.TrimSpace(*value))
	if !ok || parsed.Sign() < 0 {
		return nil, strconv.ErrSyntax
	}

	return parsed, nil
}
