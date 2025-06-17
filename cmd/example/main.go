package main

import (
	"fmt"
	"log"
	"os"
	"time"

	Ws2mqttClient "http2mqtt/pkg/Ws2mqttClient"
)

func periodic_send(client *Ws2mqttClient.MqttViaHttpClient) {

	topic := "periodic/counter" // Define the topic for periodic messages

	var i = 0
	for {
		// Publish a message
		message := fmt.Sprintf("message number %d", i)
		err := Ws2mqttClient.Publish(client, topic, []byte(message))
		if err != nil {
			log.Printf("Failed to send message: %v", err)
		}

		time.Sleep(1 * time.Second)

		i += 1
	}
}

func main() {

	defaultUrl := "ws://localhost:8080/proxy"

	var url string
	if len(os.Args) > 1 {
		url = os.Args[1]
	} else {
		url = defaultUrl
	}

	client := Ws2mqttClient.MqttViaHttp(url, func(client *Ws2mqttClient.MqttViaHttpClient, msg Ws2mqttClient.Message) {
		log.Printf("---> Received message: Topic=%s, Payload=%s", msg.Topic, msg.Payload)
		// Here you can process the message as needed
		Ws2mqttClient.Publish(client, "response/"+msg.Topic, []byte("respose to: "+string(msg.Payload)))

	})
	client.ReconnectCallback = func(client *Ws2mqttClient.MqttViaHttpClient) {
		log.Println("Reconnected to WebSocket server")
		Ws2mqttClient.Publish(client, "hello/topic", []byte("Hello from MQTT via HTTP!"))
	}

	Ws2mqttClient.Connect(&client)

	go periodic_send(&client)

	log.Println("MQTT via HTTP proxy server started")

	for {
		time.Sleep(1 * time.Second) // Keep the main function running
	}
}
