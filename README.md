# Cookies4Cuties Server (Go)

Backend API for Cookies4Cuties, built with Go, Gin, and Supabase. It handles customer orders, product creation, payment proof uploads, order management, and order confirmation emails.

The application is deployed on Vercel.

**Live API:** https://cookies4cuties-server-golang.vercel.app

**Repository:** https://github.com/jijahh/cookies4cuties_server_golang

## Tech Stack

- Go
- Gin
- Supabase (PostgreSQL and Storage)
- Upstash QStash
- SMTP
- Vercel

## Features

- Create customer orders.
- Retrieve and filter orders through admin endpoints.
- Update order status and payment verification.
- Create products with optional product images.
- Upload payment proofs for online orders.
- Validate product prices using database records.
- Send order confirmation emails through SMTP.
- Process confirmation emails asynchronously using Upstash QStash.
- Protect administrative endpoints using API key authentication.

## Project Structure

```text
cookies4cuties_server_golang/
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── database/
│   ├── handlers/
│   │   ├── order_handler.go
│   │   ├── product_handler.go
│   │   └── email_handler.go
│   ├── middleware/
│   │   └── auth.go
│   ├── routes/
│   │   └── routes.go
│   └── services/
│       ├── email.go
│       └── email_queue.go
├── .env
├── .gitignore
├── go.mod
├── go.sum
└── main.go
```

## Getting Started

### Prerequisites

- Go 1.26.5
- A Supabase project
- An SMTP email account
- An Upstash QStash account

### Installation

Clone the repository:

```bash
git clone https://github.com/jijahh/cookies4cuties_server_golang.git
```

Navigate into the project directory:

```bash
cd cookies4cuties_server_golang
```

Download dependencies:

```bash
go mod download
```

### Environment Variables

Create a `.env` file in the project root.

```env
PORT=8000

SUPABASE_URL=
SUPABASE_KEY=

ADMIN_API_KEY=

SMTP_SERVER=
SMTP_PORT=
SENDER_EMAIL=
SENDER_PASSWORD=

QSTASH_TOKEN=
QSTASH_CURRENT_SIGNING_KEY=
QSTASH_NEXT_SIGNING_KEY=

EMAIL_WORKER_URL=
```

Do not commit your `.env` file or expose its values publicly.

The following settings are currently defined directly in `config.go`:

| Setting              | Default          |
| -------------------- | ---------------- |
| Order image bucket   | `payment-proofs` |
| Product image bucket | `product-images` |
| Maximum upload size  | 5 MB             |
| Local server port    | 8000             |

Make sure the corresponding Storage buckets exist in your Supabase project.

### Running Locally

Start the application:

```bash
go run .
```

The server uses port `8000` by default, unless another port is provided through the `PORT` environment variable.

The local API will be available at:

```text
http://localhost:8000
```

## API Endpoints

### Public Endpoints

| Method | Endpoint  | Description             |
| ------ | --------- | ----------------------- |
| POST   | `/orders` | Create a customer order |

### Admin Endpoints

All admin endpoints require the `X-Admin-Key` request header.

| Method | Endpoint                  | Description                                             |
| ------ | ------------------------- | ------------------------------------------------------- |
| GET    | `/admin/orders`           | Retrieve orders with filtering, sorting, and pagination |
| POST   | `/admin/products`         | Create a product                                        |
| PATCH  | `/admin/orders/:order_id` | Update an order                                         |

### Internal Endpoint

| Method | Endpoint                      | Description                                          |
| ------ | ----------------------------- | ---------------------------------------------------- |
| POST   | `/api/tasks/send-order-email` | Receive and process queued order confirmation emails |

The email endpoint is intended for QStash callbacks and verifies the request signature.

---

## Order Management

### Create an Order

**POST `/orders`**

Content type: `multipart/form-data`

| Field              | Type        | Description                         |
| ------------------ | ----------- | ----------------------------------- |
| `customer_name`    | String      | Customer's name                     |
| `email`            | String      | Customer's email                    |
| `phone_number`     | String      | Customer's phone number             |
| `items`            | JSON string | Array of product IDs and quantities |
| `total_price`      | Decimal     | Submitted order total               |
| `pickup_location`  | String      | Selected pickup location            |
| `payment_method`   | String      | Selected payment method             |
| `proof_of_payment` | File        | Payment proof for online payments   |

Example `items` value:

```json
[
  {
    "product_id": 1,
    "quantity": 2
  },
  {
    "product_id": 2,
    "quantity": 1
  }
]
```

Allowed pickup locations:

- `pampanga`
- `katipunan`
- `rockwell`

Allowed payment methods:

- `cash_on_pickup`
- `cash_on_delivery`
- `online`

For online payments, a payment proof image is required. Cash payments must not include a payment proof.

Supported image formats are JPEG, PNG, and WebP. The maximum upload size is 5 MB.

The backend retrieves product information from Supabase and calculates the actual order total using database prices instead of relying on the submitted total.

New orders are saved with the following initial values:

- `order_status`: `pending`
- `payment_verified`: `false`

Example request:

```bash
curl -X POST http://localhost:8000/orders \
  -F "customer_name=John Doe" \
  -F "email=john@example.com" \
  -F "phone_number=09123456789" \
  -F 'items=[{"product_id":1,"quantity":2}]' \
  -F "total_price=200" \
  -F "pickup_location=pampanga" \
  -F "payment_method=cash_on_pickup"
```

The response includes the created order and an `email_queued` boolean indicating whether the email job was successfully submitted to QStash.

### Retrieve Orders

**GET `/admin/orders`**

Requires the `X-Admin-Key` header.

Supported query parameters:

| Parameter          | Description                          |
| ------------------ | ------------------------------------ |
| `order_status`     | Filter by order status               |
| `customer_name`    | Search by customer name              |
| `payment_method`   | Filter by payment method             |
| `payment_verified` | Filter by payment verification       |
| `email`            | Search by customer email             |
| `pickup_location`  | Filter by pickup location            |
| `created_from`     | Filter orders from a date (RFC3339)  |
| `created_to`       | Filter orders until a date (RFC3339) |
| `min_total`        | Minimum order total                  |
| `max_total`        | Maximum order total                  |
| `sort_by`          | Column used for sorting              |
| `sort_order`       | `asc` or `desc`                      |
| `limit`            | Number of records to return          |
| `offset`           | Number of records to skip            |

Allowed sorting columns:

- `created_at`
- `total_price`
- `order_status`
- `email`
- `pickup_location`
- `payment_method`

Default sorting is `created_at` descending.

The default limit is 50 records, with a maximum of 200.

Example:

```bash
curl -X GET "http://localhost:8000/admin/orders?order_status=pending&limit=20&offset=0" \
  -H "X-Admin-Key: YOUR_ADMIN_API_KEY"
```

### Update an Order

**PATCH `/admin/orders/:order_id`**

Requires the `X-Admin-Key` header.

Accepts form fields:

| Field              | Type    | Description                 |
| ------------------ | ------- | --------------------------- |
| `order_status`     | String  | New order status            |
| `payment_verified` | Boolean | Payment verification status |

Both fields are optional, but at least one must be provided.

Example:

```bash
curl -X PATCH http://localhost:8000/admin/orders/1 \
  -H "X-Admin-Key: YOUR_ADMIN_API_KEY" \
  -F "order_status=completed" \
  -F "payment_verified=true"
```

## Product Management

### Create a Product

**POST `/admin/products`**

Requires the `X-Admin-Key` header.

Content type: `multipart/form-data`

| Field             | Type    | Description                                      |
| ----------------- | ------- | ------------------------------------------------ |
| `product_name`    | String  | Product name (maximum 120 characters)            |
| `unit_price`      | Decimal | Product price, greater than zero                 |
| `product_details` | String  | Optional product description                     |
| `active`          | Boolean | Whether the product is active (defaults to true) |
| `image`           | File    | Optional product image                           |

Supported image formats are JPEG, PNG, and WebP. The maximum upload size is 5 MB.

Example:

```bash
curl -X POST http://localhost:8000/admin/products \
  -H "X-Admin-Key: YOUR_ADMIN_API_KEY" \
  -F "product_name=Chocolate Chip Cookies" \
  -F "unit_price=100" \
  -F "product_details=Freshly baked chocolate chip cookies" \
  -F "active=true"
```

## Email Processing

Order confirmation emails are handled asynchronously using Upstash QStash.

The process works as follows:

1. A customer submits an order.
2. The backend validates the order and saves it to Supabase.
3. The backend creates an email payload containing the recipient, order status, pickup location, payment method, items, and total price.
4. The payload is published to QStash.
5. The order endpoint returns its response without waiting for the email to be sent.
6. QStash sends a request to `/api/tasks/send-order-email`.
7. The email handler verifies the QStash signature.
8. The email service sends the confirmation email through SMTP.

QStash is configured to retry failed deliveries up to three times.

The email service uses an HTML email template containing the order summary and receipt information.

## Authentication

Administrative endpoints are protected using an API key middleware.

Include the following header in admin requests:

```text
X-Admin-Key: YOUR_ADMIN_API_KEY
```

The email worker endpoint uses QStash signature verification instead of the admin API key.

Keep all API keys, SMTP credentials, Supabase credentials, and QStash signing keys private.

## Testing

Run the Go test command:

```bash
go test ./...
```

You can also test the API endpoints using Postman, Insomnia, or `curl`.

For admin endpoints, make sure to include the `X-Admin-Key` header.

For order creation and product creation, use `multipart/form-data`.

For the email worker, test through QStash rather than sending unsigned requests directly to the endpoint.

## Deployment

The application is deployed on Vercel.

Live URL:

https://cookies4cuties-server-golang.vercel.app

Configure the required environment variables in the Vercel project settings. Do not upload your local `.env` file.

Changes pushed to the connected GitHub repository can be deployed through Vercel's Git integration.
