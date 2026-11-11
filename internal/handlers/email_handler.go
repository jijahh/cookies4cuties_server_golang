package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/upstash/qstash-go"

	"github.com/jijahh/cookies4cuties_server_golang/internal/services"
)

type EmailHandler struct {
	EmailService      *services.EmailService
	CurrentSigningKey string
	NextSigningKey    string
	WorkerURL         string
}

func NewEmailHandler(
	emailService *services.EmailService,
	currentSigningKey string,
	nextSigningKey string,
	workerURL string,
) *EmailHandler {
	return &EmailHandler{
		EmailService:      emailService,
		CurrentSigningKey: currentSigningKey,
		NextSigningKey:    nextSigningKey,
		WorkerURL:         workerURL,
	}
}

func (h *EmailHandler) SendOrderEmail(c *gin.Context) {
	// Read the raw request body first.
	body, err := io.ReadAll(
		io.LimitReader(c.Request.Body, 1<<20),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Could not read email job.",
		})
		return
	}

	// Verify that the request actually came from QStash.
	signature := c.GetHeader("Upstash-Signature")

	receiver := qstash.NewReceiver(
		h.CurrentSigningKey,
		h.NextSigningKey,
	)

	err = receiver.Verify(qstash.VerifyOptions{
		Signature: signature,
		Body:      string(body),
		Url:       h.WorkerURL,
	})

	if err != nil {
		log.Printf("Invalid QStash signature: %v", err)

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid QStash signature.",
		})
		return
	}

	// Decode the email data.
	var emailData services.OrderEmailData

	if err := json.Unmarshal(body, &emailData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid email job payload.",
		})
		return
	}

	// Actually send the email.
	if err := h.EmailService.SendOrderEmail(emailData); err != nil {
		log.Printf("Failed to send order email: %v", err)

		// Non-2xx response tells QStash to retry.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to send email.",
		})
		return
	}

	// Successful response tells QStash the job completed.
	c.Status(http.StatusNoContent)
}
