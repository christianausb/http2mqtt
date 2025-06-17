package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"

	utils "http2mqtt/pkg/utils"
)

var DEVICE_TOPIC_PREFIX = getEnv("HTTP2MQTT_DEVICE_TOPIC_PREFIX", "device")

var WEBSOCKET_HTTP_PATH = getEnv("HTTP2MQTT_WEBSOCKET_HTTP_PATH", "/proxy")
var WEBSSOCKET_LISTENER = getEnv("HTTP2MQTT_WEBSOCKET_LISTENER", ":8080")

var HTTP_HEADER = getEnv("HTTP2MQTT_HTTP_HEADER", "X-Main-Topic")
var HTTP_HEADER_PREFIX = getEnv("HTTP2MQTT_HTTP_HEADER_PREFIX", "CN=")

var MQTT_BROKER_URL = getEnv("HTTP2MQTT_MQTT_BROKER_URL", "tcp://localhost:1883")
var MQTT_BROKER_USERNAME = getEnv("HTTP2MQTT_MQTT_BROKER_USERNAME", "")
var MQTT_BOKER_PASSWORD = getEnv("HTTP2MQTT_MQTT_BROKER_PASSWORD", "")

// (sub)topics to forward from the cloud to the device
var CLOUD_2_DEVICE_TOPICS = getJsonEncodedListOfStringsFromEnv("HTTP2MQTT_CLOUD_2_DEVICE_TOPICS", `["set/value", "status/update", "command/execute"]`)

func printSettings() {
	log.Println("HTTP2MQTT_* settings:")

	log.Println("  DEVICE_TOPIC_PREFIX:", DEVICE_TOPIC_PREFIX)
	log.Println("  WEBSOCKET_HTTP_PATH:", WEBSOCKET_HTTP_PATH)
	log.Println("  WEBSSOCKET_LISTENER:", WEBSSOCKET_LISTENER)

	log.Println("  HTTP_HEADER:", HTTP_HEADER)
	log.Println("  HEADER_PREFIX:", HTTP_HEADER_PREFIX)

	log.Println("  MQTT_BROKER_URL:", MQTT_BROKER_URL)
	log.Println("  MQTT_BROKER_USERNAME:", MQTT_BROKER_USERNAME)
	log.Printf("  MQTT_BROKER_PASSWORD: length=%d\n", len(MQTT_BOKER_PASSWORD))

	log.Println("  CLOUD_2_DEVICE_TOPICS:", CLOUD_2_DEVICE_TOPICS)
}

func getEnv(envVariable, defaultValue string) string {
	value, exists := os.LookupEnv(envVariable)
	if !exists {
		return defaultValue
	}
	return value
}

func getJsonEncodedListOfStringsFromEnv(envVariable string, defaultValue string) []string {

	topicsJSON := getEnv(envVariable, defaultValue)
	var topics []string
	if err := json.Unmarshal([]byte(topicsJSON), &topics); err != nil {
		log.Fatalf("Failed to parse %s into list of strings: %v", envVariable, err)
		return []string{} // Return an empty slice if parsing fails
	}
	return topics
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for simplicity; adjust as needed for security
		return true
	},
}

func parseEnginxProxyHeader(r *http.Request) (int64, error) {
	// Extract the main topic from HTTP headers
	headerStr := r.Header.Get(HTTP_HEADER)
	if headerStr == "" {
		return -1, fmt.Errorf("missing %s header", HTTP_HEADER)
	}

	// Assuming the header is in the format "CN=62738482", we can extract the serial number
	if !strings.HasPrefix(headerStr, "CN=") {
		return -1, fmt.Errorf("invalid %s header format", HTTP_HEADER)
	}

	deviceSerialAsString := strings.Replace(headerStr, HTTP_HEADER_PREFIX, "", 1) // Remove the "CN=" prefix if it exists
	deviceSerial, err := strconv.Atoi(deviceSerialAsString)
	if err != nil {
		return -1, fmt.Errorf("error parsing device serial: %s", err)
	}

	return int64(deviceSerial), nil
}

func setUpMqtt2Websocker(topicsCloud2Device []string, deviceSerial int64, mqttClient *mqtt.Client, conn *websocket.Conn) {

	for _, subtopic := range topicsCloud2Device {

		if strings.HasPrefix(subtopic, "/") {
			log.Printf("invalid subtopic: %s. Subtopics must not start with '/'.", subtopic)
			continue
		}

		mainTopic := fmt.Sprintf("%s/%d/", DEVICE_TOPIC_PREFIX, deviceSerial) // e.g. "device/62738482/"
		topic := fmt.Sprintf("%s%s", mainTopic, subtopic)                     // e.g. "device/62738482/some/subtopic"

		if token := (*mqttClient).Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {

			localTopic := strings.TrimPrefix(msg.Topic(), mainTopic) // yields e.g. "some/subtopic"

			encodedPacket, encodeErr := utils.EncodePacket(localTopic, msg.Payload())
			if encodeErr != nil {
				log.Println("encode error:", encodeErr)
				return
			}

			// Forward MQTT messages to the WebSocket connection
			err := conn.WriteMessage(websocket.BinaryMessage, encodedPacket)
			if err != nil {
				log.Println("WebSocket write error:", err)
			}

		}); token.Wait() && token.Error() != nil {
			log.Println("MQTT subscription error for topic", topic, ":", token.Error())
			return
		}
	}
}

func loopWebsocket2Mqtt(websocketConnection *websocket.Conn, deviceSerial int64, mqttClient *mqtt.Client) {
	for {
		messageType, message, err := websocketConnection.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		// Check if the message is an MQTT packet
		if messageType == websocket.BinaryMessage {

			// Decode the MQTT packet (assuming decodePacket is defined elsewhere)
			topic, payload, decodeError := utils.DecodePacket(message)
			if decodeError != nil {
				log.Println("decode error:", decodeError)
				break
			}
			// Forward the MQTT packet to the broker
			topic = fmt.Sprintf("%s/%d/%s", DEVICE_TOPIC_PREFIX, deviceSerial, topic) // Prepend the device serial to the topic
			if token := (*mqttClient).Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
				log.Println("MQTT publish error:", token.Error())
			}
		}
	}
}

func buildMqttOptions(deviceSerial int64) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions().AddBroker(MQTT_BROKER_URL)
	opts.SetClientID(fmt.Sprintf("http2mqtt-%d", deviceSerial))
	opts.SetUsername(MQTT_BROKER_USERNAME)
	opts.SetPassword(MQTT_BOKER_PASSWORD)
	opts.SetAutoReconnect(true)

	return opts
}

func handleNewWebsocketConnection(w http.ResponseWriter, r *http.Request) {

	// Extract the serial number of the connecting device from HTTP headers
	deviceSerial, err := parseEnginxProxyHeader(r)
	if err != nil {
		log.Println("Error parsing device serial:", err)
		http.Error(w, "Invalid device serial", http.StatusBadRequest)
		return
	}

	log.Printf("Device with serial %d connected", deviceSerial)

	// Upgrade the HTTP connection to a WebSocket connection
	websocketConnection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer websocketConnection.Close()

	// Create a new MQTT client for this connection
	opts := buildMqttOptions(deviceSerial)

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost for device serial %d: %v", deviceSerial, err)
		// nothing here; maybe close the websocker connection?
	})

	opts.OnConnect = func(client mqtt.Client) {
		log.Printf("MQTT reconnected for device serial %d", deviceSerial)

		// subscribe to the given topics and forward messages via Websocket
		setUpMqtt2Websocker(CLOUD_2_DEVICE_TOPICS, deviceSerial, &client, websocketConnection)
	}

	// connect to MQTT broker
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Println("MQTT connection error:", token.Error())
		return
	}
	defer mqttClient.Disconnect(250)

	// Set a close handler to detect when the WebSocket connection is closed
	websocketConnection.SetCloseHandler(func(code int, text string) error {
		log.Printf("WebSocket connection closed for device serial %d: code=%d, text=%s", deviceSerial, code, text)
		mqttClient.Disconnect(250)
		return nil
	})

	// Handle incoming WebSocket messages and publish them to MQTT
	loopWebsocket2Mqtt(websocketConnection, deviceSerial, &mqttClient)
}

func validate() bool {

	var ok = true

	for _, subtopic := range CLOUD_2_DEVICE_TOPICS {

		if strings.HasPrefix(subtopic, "/") {
			log.Fatalf("Invalid subtopic: %s. Subtopics must not start with '/'.", subtopic)
			ok = false
			continue
		}
	}

	return ok
}

func main() {

	printSettings()

	if !validate() {
		log.Fatal("Invalid configuration. Exiting.")
		return
	}

	http.HandleFunc(WEBSOCKET_HTTP_PATH, handleNewWebsocketConnection)
	log.Printf("HTTP to MQTT proxy server started on %s\n", WEBSSOCKET_LISTENER)
	log.Fatal(http.ListenAndServe(WEBSSOCKET_LISTENER, nil))
}
