# Go Microservice Project

A small Go microservices demo for managing a service catalog and customer
bookings. The services use HTTP/JSON for their public APIs, gRPC for an
internal catalog lookup, PostgreSQL for persistence, and RabbitMQ to publish
booking-created events to a notification worker.

## What the project does

The project is split into three runnable Go programs:

| Component | Purpose | Address |
| --- | --- | --- |
| Catalog service | Creates and lists services over HTTP, and answers internal service-existence checks over gRPC. | HTTP `:3002`, gRPC `:50051` |
| Booking service | Creates and lists bookings. Before creating a booking, it verifies the referenced service with the Catalog service and publishes a booking event. | HTTP `:3001` |
| Notification worker | Consumes `booking.created` events from RabbitMQ and logs a notification message. | Background worker; no HTTP endpoint |

### Booking creation flow

1. A client submits a booking to the Booking service.
2. The Booking service validates the request and calls
   `CatalogService.CheckServiceExists` over gRPC (with a two-second timeout).
3. If the service exists, the booking is inserted into PostgreSQL with status
   `pending`.
4. The Booking service publishes a persistent JSON message to the
   `booking.created` RabbitMQ queue.
5. The Notification worker consumes the event, logs it, and acknowledges the
   message.

## Repository layout

```text
cmd/
  booking/main.go       Booking HTTP API
  catalog/main.go       Catalog HTTP API and gRPC server
  notification/main.go  RabbitMQ notification worker
  services/main.go      Combined/experimental entrypoint (currently incomplete)
proto/
  catalog.proto         Catalog gRPC service definition
  catalog.pb.go         Generated protobuf message code
  catalog_grpc.pb.go    Generated gRPC client/server code
sql/
  schema.sql            PostgreSQL table definitions
docker-compose.yml      Local PostgreSQL and RabbitMQ dependencies
```

## Requirements

- Go `1.26.0` (as declared in `go.mod`)
- Docker and Docker Compose

## Setup and run

### 1. Start the infrastructure

From the repository root:

```sh
docker compose up -d
```

This starts:

- PostgreSQL on `localhost:5435` (container port `5432`)
- RabbitMQ on `localhost:5672`
- RabbitMQ Management UI on `http://localhost:15672`

The Compose configuration creates a `homefix` database and user, and mounts
`sql/schema.sql` as PostgreSQL's initialization script. PostgreSQL applies that
script when initializing a new data directory.

Local Compose credentials are `homefix` / `homefix` for both PostgreSQL and
RabbitMQ. These are development defaults, not production credentials.

### 2. Run the services

Open separate terminals in the repository root. Start Catalog first so its
HTTP and gRPC servers are available, then start the other programs:

```sh
go run ./cmd/catalog
```

```sh
go run ./cmd/notification
```

```sh
go run ./cmd/booking
```

The services currently use local connection addresses and fixed listen ports
in their Go source; they do not read these settings from environment variables.
If you change the Compose ports or run dependencies on another host, update
the corresponding connection strings in `cmd/catalog/main.go`,
`cmd/booking/main.go`, and `cmd/notification/main.go`.

Stop the infrastructure when finished:

```sh
docker compose down
```

## HTTP API

All endpoints accept and return JSON. The Catalog API listens on port `3002`;
the Booking API listens on port `3001`.

### Catalog

#### `GET /services`

Returns all catalog services, ordered by ID.

Example response (`200 OK`):

```json
[
  {
    "id": 1,
    "name": "Plumbing"
  }
]
```

An empty catalog returns `[]`.

#### `POST /services`

Creates a service. `name` must contain non-whitespace text; surrounding
whitespace is trimmed.

Request:

```json
{
  "name": "Plumbing"
}
```

Example response (`201 Created`):

```json
{
  "id": 1,
  "name": "Plumbing"
}
```

Invalid or missing names return `400 Bad Request`.

### Bookings

#### `GET /bookings`

Returns all bookings, ordered by ID.

Example response (`200 OK`):

```json
[
  {
    "id": 1,
    "service_id": 1,
    "customer_name": "Aarav Sharma",
    "status": "pending",
    "created_at": "2026-01-15T10:30:00Z"
  }
]
```

An empty booking list returns `[]`.

#### `POST /bookings`

Creates a booking for an existing catalog service. `service_id` must be a
positive integer and `customer_name` must contain non-whitespace text;
surrounding whitespace in the name is trimmed.

Request:

```json
{
  "service_id": 1,
  "customer_name": "Aarav Sharma"
}
```

Example response (`201 Created`):

```json
{
  "id": 1,
  "service_id": 1,
  "customer_name": "Aarav Sharma",
  "status": "pending",
  "created_at": "2026-01-15T10:30:00Z"
}
```

Possible errors:

| Status | Meaning |
| --- | --- |
| `400 Bad Request` | Invalid/missing fields, or the requested service does not exist. |
| `503 Service Unavailable` | The Catalog gRPC service could not be reached. |
| `500 Internal Server Error` | A database or event-publishing operation failed. |

### Example requests

```sh
curl -X POST http://localhost:3002/services \
  -H "Content-Type: application/json" \
  -d '{"name":"Plumbing"}'

curl http://localhost:3002/services

curl -X POST http://localhost:3001/bookings \
  -H "Content-Type: application/json" \
  -d '{"service_id":1,"customer_name":"Aarav Sharma"}'

curl http://localhost:3001/bookings
```

## Internal gRPC API

The Catalog service also exposes the gRPC service defined in
`proto/catalog.proto` on port `50051`.

```protobuf
service CatalogService {
  rpc CheckServiceExists (CheckServiceRequest)
      returns (CheckServiceResponse);
}
```

`CheckServiceExists` accepts a `service_id` and returns `exists: true` when a
matching service row is present in PostgreSQL. The Booking service uses this
method before inserting a booking.

## Data model and messaging

`sql/schema.sql` defines two tables:

- `services`: generated `id` and required `name`.
- `bookings`: generated `id`, `service_id`, required `customer_name`,
  `status` (default `pending`), and `created_at` (default current time).

Bookings reference catalog service IDs in application logic; the current
schema does not declare a database foreign-key constraint.

The Booking service declares the durable RabbitMQ queue `booking.created` and
publishes JSON messages with this shape:

```json
{
  "booking_id": 1,
  "service_id": 1,
  "customer_name": "Aarav Sharma"
}
```

The Notification worker consumes this queue and logs the booking details. It
does not currently send an email, SMS, or external notification.

## Current scope

This is a compact learning/demo project. The current APIs provide create and
list operations only; authentication, booking updates/cancellation,
pagination, automated migrations, and configurable environment-based
deployment settings are not implemented.

`cmd/services` is an additional combined/experimental entrypoint and is not
used by the setup instructions; it currently fails to compile. Use the three
individual entrypoints listed above.
