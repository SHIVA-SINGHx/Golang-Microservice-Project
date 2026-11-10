package main

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type bookingCreated struct {
	BookingID    int64  `json:"booking_id"`
	ServiceID    int64  `json:"service_id"`
	CustomerName string `json:"customer_name"`
}

func main() {
	conn, err := amqp.Dial("amqp://homefix:homefix@localhost:5672/")
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}

	defer ch.Close()

	_, err = ch.QueueDeclare("booking.created", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume("booking.created", "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Waiting for booking created")

	for msg := range msgs {
		var event bookingCreated

		if err := json.Unmarshal(msg.Body, &event); err != nil {
			log.Println(err)
			continue
		}

		log.Printf("Notification for booking #%d created for %s (service id=%d)", event.BookingID, event.CustomerName, event.ServiceID)

		if err := msg.Ack(false); err != nil {
			log.Println(err)
		}
	}
}
