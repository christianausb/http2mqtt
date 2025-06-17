package Ws2mqttClient

import (
	"fmt"
	"log"
	"net/http"
	"time"

	utils "http2mqtt/pkg/utils"

	"github.com/gorilla/websocket"
)

type Message struct {
	Topic   string
	Payload []byte
}

func sendMessage(websocketConnection *websocket.Conn, topic string, message []byte) error {

	encodedPacket, err := utils.EncodePacket(topic, []byte(message)) // Assuming encodePacket is defined elsewhere
	if err != nil {
		log.Printf("Failed to encode message: %v", err)
		return err
	}

	err = websocketConnection.WriteMessage(websocket.BinaryMessage, encodedPacket)
	if err != nil {
		log.Printf("Failed to send message: %v", err)
		return err
	}

	return nil
}

func readLoopWebsocket(client *MqttViaHttpClient) {
	// Read incoming messages
	for {
		_, msg, err := (*client).websocketConnection.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		// Process the received message
		topic, payload, err := utils.DecodePacket(msg) // Assuming decodePacket is defined elsewhere
		if err != nil {
			log.Printf("Failed to decode message: %v", err)
			continue
		}

		tmp := Message{
			Topic:   topic,
			Payload: payload,
		}

		(*client).MessageCallback(client, tmp)
	}
}

func connectWebSocket(client *MqttViaHttpClient) {
	headers := make(http.Header)
	headers.Set("X-Main-Topic", "CN=62738482")

	// Monitor the WebSocket connection and trigger the callback on reconnect
	for {
		if (*client).websocketConnection == nil || (*client).websocketConnection.CloseHandler() != nil {
			log.Println("WebSocket connection lost, attempting to reconnect...")
			time.Sleep(2 * time.Second) // Wait before attempting to reconnect

			var err error = nil
			(*client).websocketConnection, _, err = websocket.DefaultDialer.Dial((*client).Url, headers)
			if err != nil {
				log.Printf("Reconnection attempt failed: %v", err)
				continue
			}

			(*client).ReconnectCallback(client) // Call the reconnect callback if defined

			readLoopWebsocket(client)
		}
		time.Sleep(1 * time.Second) // Check connection status periodically
	}

	// defer websocketConnection.Close()

}

type MqttViaHttpClient struct {
	Url                 string
	websocketConnection *websocket.Conn // WebSocket connection
	MessageCallback     func(client *MqttViaHttpClient, msg Message)
	ReconnectCallback   func(client *MqttViaHttpClient)
}

func Publish(client *MqttViaHttpClient, topic string, message []byte) error {

	if (*client).websocketConnection == nil {
		log.Println("WebSocket connection is not established. Cannot publish message.")
		return fmt.Errorf("websocket connection is not established")
	}

	err := sendMessage((*client).websocketConnection, topic, []byte(message))
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	}

	return err
}

func Connect(client *MqttViaHttpClient) {

	log.Println("connecting to ", (*client).Url)
	go connectWebSocket(client)
}

func MqttViaHttp(url string, callback func(client *MqttViaHttpClient, msg Message)) MqttViaHttpClient {

	var client = MqttViaHttpClient{
		Url:                 url,
		websocketConnection: nil, // Initially no WebSocket connection
		MessageCallback:     callback,
	}

	return client

}
