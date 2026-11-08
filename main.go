package main

import (
	"context"
	"log"
	"net"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	catalogpb "example.com/homefix-microservices/proto"
)

type catalogServer struct {
	catalogpb.UnimplementedCatalogServiceServer

	pool *pgxpool.Pool
}

func (s *catalogServer) CheckServiceExists(ctx context.Context, req *catalogpb.CheckServiceRequest) (*catalogpb.CheckServiceResponse, error) {
	var exists bool

	err := s.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM services WHERE id = $1)",
		req.GetServiceId(),
	).Scan(&exists)

	if err != nil {
		return nil, err
	}

	return &catalogpb.CheckServiceResponse{Exists: exists}, nil
}

type service struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type serviceRequest struct {
	Name string `json:"name"`
}

func main() {
	// create postgresSQL connection pool
	pool, err := pgxpool.New(context.Background(), "postgres://homefix:homefix@localhost:5435/homefix?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	// check if postgres is reachable
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Get("/services", func(c fiber.Ctx) error {
		rows, err := pool.Query(context.Background(), "SELECT id, name FROM services ORDER BY id")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}

		defer rows.Close() // release query resources when its finished

		list := []service{}

		for rows.Next() {
			var s service

			if err := rows.Scan(&s.ID, &s.Name); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
			}

			list = append(list, s)
		}

		// check if any error is there while u read the rows

		if err := rows.Err(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}

		return c.JSON(list)

	})

	app.Post("/services", func(c fiber.Ctx) error {
		var body serviceRequest

		if err := c.Bind().Body(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
		}

		var s service

		err := pool.QueryRow(context.Background(),
			"INSERT INTO services (name) VALUES ($1) RETURNING id, name",
			strings.TrimSpace(body.Name),
		).Scan(&s.ID, &s.Name)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(
				fiber.Map{"error": "database error"},
			)
		}

		return c.Status(fiber.StatusCreated).JSON(s)
	})

	// open a TCP listener on a diff port - 50051

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	catalogpb.RegisterCatalogServiceServer(grpcServer, &catalogServer{pool: pool})

	go func() {
		log.Fatal(grpcServer.Serve(lis))
	}()

	// start our http server on one particular port: 3002
	log.Fatal(app.Listen(":3002"))

}
