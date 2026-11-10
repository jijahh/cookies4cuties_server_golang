package services

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type EmailService struct {
	SMTP_SERVER     string
	SMTP_PORT       string
	SENDER_EMAIL    string
	SENDER_PASSWORD string
}

type ReceiptItem struct {
	Name     string
	Quantity int
	Subtotal decimal.Decimal
}

type OrderEmailData struct {
	RecipientEmail string
	OrderStatus    string
	PickupLocation string
	PaymentMethod  string
	Items          []ReceiptItem
	TotalPrice     decimal.Decimal
}

func NewEmailService(
	server string,
	port string,
	senderEmail string,
	senderPassword string,
) *EmailService {
	return &EmailService{
		SMTP_SERVER:     server,
		SMTP_PORT:       port,
		SENDER_EMAIL:    senderEmail,
		SENDER_PASSWORD: senderPassword,
	}
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 0; background-color: #f7fafc;">
    <table border="0" cellpadding="0" cellspacing="0" width="100%"
        style="background-color: #f7fafc; padding: 40px 20px;">
        <tr>
            <td align="center">
                <table border="0" cellpadding="0" cellspacing="0" width="100%"
                    style="max-width: 600px; background-color: #ffffff; border-radius: 12px; overflow: hidden;">

                    <tr>
                        <td align="center" style="background-color: #fed7aa; padding: 30px 20px;">
                            <h1 style="margin: 0; font-family: Arial, sans-serif; font-size: 28px; color: #7c2d12;">
                                🍪 Cookies4Cuties
                            </h1>
                        </td>
                    </tr>

                    <tr>
                        <td style="padding: 40px 30px;">
                            <h2 style="font-family: sans-serif; color: #1a202c;">
                                Thank you for your order!
                            </h2>

                            <p style="font-family: sans-serif; font-size: 16px; line-height: 1.6; color: #4a5568;">
                                We're baking your treats with love. Below is the summary of your order
                                and the details for your upcoming pick-up.
                            </p>

                            <table border="0" cellpadding="0" cellspacing="0" width="100%"
                                style="margin-bottom: 30px; border-collapse: collapse;">
                                <thead>
                                    <tr style="background-color: #f8fafc;">
                                        <th align="left" style="padding: 12px;">Item</th>
                                        <th align="center" style="padding: 12px;">Qty</th>
                                        <th align="right" style="padding: 12px;">Total</th>
                                    </tr>
                                </thead>

                                <tbody>
                                    {{range .Items}}
                                    <tr>
                                        <td style="padding: 12px; border-bottom: 1px solid #eeeeee; color: #4a5568;">
                                            {{.Name}}
                                        </td>
                                        <td align="center" style="padding: 12px; border-bottom: 1px solid #eeeeee; color: #4a5568;">
                                            {{.Quantity}}
                                        </td>
                                        <td align="right" style="padding: 12px; border-bottom: 1px solid #eeeeee; color: #4a5568;">
                                            ₱{{.Subtotal}}
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>

                            <h3 style="text-align: right; font-family: sans-serif; color: #7c2d12;">
                                Total: ₱{{.TotalPrice}}
                            </h3>

                            <table border="0" cellpadding="0" cellspacing="0" width="100%"
                                style="background-color: #fff7ed; border-radius: 8px; padding: 20px; margin-bottom: 24px;">
                                <tr>
                                    <td style="padding-bottom: 10px; color: #7c2d12;">
                                        <strong>Order Status:</strong>
										<span style="text-transform: capitalize; background-color: #ffedd5; padding: 2px 8px; border-radius: 4px; font-weight: bold;">
											{{.OrderStatus}}
										</span>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding-bottom: 10px; color: #7c2d12;">
                                        <strong>Pick-up Location:</strong>
										<span style="text-transform: capitalize;">
											{{.PickupLocation}}
										</span>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="color: #7c2d12;">
                                        <strong>Payment Method:</strong>
										<span style="text-transform: capitalize;">
											{{.PaymentMethod}}
										</span>
                                    </td>
                                </tr>
                            </table>

                            <p style="font-family: sans-serif; font-size: 15px; line-height: 1.6; color: #4a5568;">
                                If you have any changes or questions regarding your order, reply directly
                                to this email. Also, if your mode of payment is COD, please reply with
                                <strong>"Yes"</strong> so that we can confirm your order. See you soon!
                            </p>
                        </td>
                    </tr>

                    <tr>
                        <td align="center" style="background-color: #f8fafc; padding: 20px; border-top: 1px solid #e2e8f0;">
                            <p style="font-family: sans-serif; font-size: 12px; color: #94a3b8;">
                                © 2026 Cookies4Cuties. All rights reserved.
                            </p>
                        </td>
                    </tr>

                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`

func (s *EmailService) SendOrderEmail(data OrderEmailData) error {
	// Generate HTML body.
	tmpl, err := template.New("order-email").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parse email template: %w", err)
	}

	var htmlBody bytes.Buffer

	if err := tmpl.Execute(&htmlBody, data); err != nil {
		return fmt.Errorf("render HTML email: %w", err)
	}

	// Generate plain-text fallback.
	var textBody strings.Builder

	fmt.Fprintf(&textBody, "Hello!\n\n")
	fmt.Fprintf(&textBody, "Thank you for ordering from Cookies4Cuties.\n\n")
	fmt.Fprintf(&textBody, "We've received your order.\n\n")
	fmt.Fprintf(&textBody, "ORDER SUMMARY\n")
	fmt.Fprintf(&textBody, "----------------------------\n\n")

	for _, item := range data.Items {
		fmt.Fprintf(
			&textBody,
			"%s: %d pcs | Total: PHP %s\n",
			item.Name,
			item.Quantity,
			item.Subtotal.StringFixed(2),
		)
	}

	fmt.Fprintf(&textBody, "\nTotal: PHP %s\n\n", data.TotalPrice.StringFixed(2))

	fmt.Fprintf(&textBody, "ORDER DETAILS\n")
	fmt.Fprintf(&textBody, "----------------------------\n\n")
	fmt.Fprintf(&textBody, "Order Status: %s\n", data.OrderStatus)
	fmt.Fprintf(&textBody, "Pickup Location: %s\n", data.PickupLocation)
	fmt.Fprintf(&textBody, "Payment Method: %s\n\n", data.PaymentMethod)

	fmt.Fprintf(
		&textBody,
		"If you have questions regarding your order, you can reply directly to this email.\n\n",
	)

	fmt.Fprintf(&textBody, "Thank you for supporting Cookies4Cuties!\n")

	// Construct multipart email.
	var message bytes.Buffer

	boundary := fmt.Sprintf("cookies4cuties-%d", time.Now().UnixNano())

	headers := textproto.MIMEHeader{}
	headers.Set("From", fmt.Sprintf("Cookies4Cuties <%s>", s.SENDER_EMAIL))
	headers.Set("To", data.RecipientEmail)
	headers.Set("Subject", fmt.Sprintf("🍪 Your Cookies4Cuties Order is %s!", strings.Title(data.OrderStatus)))
	headers.Set("Date", time.Now().Format(time.RFC1123Z))
	headers.Set("Reply-To", s.SENDER_EMAIL)
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))

	for key, values := range headers {
		for _, value := range values {
			fmt.Fprintf(&message, "%s: %s\r\n", key, value)
		}
	}

	fmt.Fprintf(&message, "\r\n")

	// Plain-text version.
	fmt.Fprintf(&message, "--%s\r\n", boundary)
	fmt.Fprintf(&message, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&message, "Content-Transfer-Encoding: 8bit\r\n\r\n")
	fmt.Fprintf(&message, "%s\r\n", textBody.String())

	// HTML version.
	fmt.Fprintf(&message, "--%s\r\n", boundary)
	fmt.Fprintf(&message, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&message, "Content-Transfer-Encoding: 8bit\r\n\r\n")
	fmt.Fprintf(&message, "%s\r\n", htmlBody.String())

	fmt.Fprintf(&message, "--%s--\r\n", boundary)

	// Connect to SMTP server.
	auth := smtp.PlainAuth(
		"",
		s.SENDER_EMAIL,
		s.SENDER_PASSWORD,
		s.SMTP_SERVER,
	)

	address := s.SMTP_SERVER + ":" + s.SMTP_PORT

	err = smtp.SendMail(
		address,
		auth,
		s.SENDER_EMAIL,
		[]string{data.RecipientEmail},
		message.Bytes(),
	)

	if err != nil {
		return fmt.Errorf("send order email: %w", err)
	}

	return nil
}
