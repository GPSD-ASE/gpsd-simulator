package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	broker = "localhost"
	port   = 1883

	dublinLatitude  = 53.350140
	dublinLongitude = -6.266155

	incidentChannel = "gpsd/incident"
	ertChannel      = "gpsd/location"
)

type LocationPayload struct {
	Id        string `json:"id"`
	FireTruck Point  `json:"firetruck"`
	Ambulance Point  `json:"ambulance"`
	PatrolCar Point  `json:"patrolcar"`
}

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type Route []Point

type ERTType uint

const (
	FIRE_TRUCK ERTType = iota
	AMBULANCE  ERTType = iota
	PATROL_CAR ERTType = iota
)

type ERTMember struct {
	ERTType          ERTType
	currentLocation  Point
	destination      Point
	destinationRoute Route
	destRtIdx        int
	staticRoute      Route
	staticRtIdx      int
	reached          bool
}

type ERT struct {
	ID        string
	fireTruck ERTMember
	ambulance ERTMember
	patrolCar ERTMember
}

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	fmt.Printf("Topic: %s Message: %s\n", msg.Topic(), msg.Payload())

	switch msg.Topic() {
	case ertChannel:
	case incidentChannel:
	default:
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Printf("Connect lost: %v\n", err)
}

func subscribe(client mqtt.Client) {
	token := client.Subscribe(ertChannel, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic %s\n", ertChannel)

	token = client.Subscribe(incidentChannel, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic %s\n", incidentChannel)
}

func publish(client mqtt.Client) {
	text := fmt.Sprintf("Message 1")
	token := client.Publish(incidentChannel, 0, false, text)
	token.Wait()
	time.Sleep(time.Second)
}

func randomPt() Point {
	return Point{
		dublinLatitude + (-1 + rand.Float64()*2),
		dublinLongitude + (-1 + rand.Float64()*2),
	}
}

func genStaticRoute(count int) Route {
	pts := make([]Point, 0, count)

	for i := 0; i < count; i++ {
		pts = append(pts, randomPt())
	}

	return pts
}

func generateDestinationRoute(source, dest Point, numberOfPts int) Route {
	pts := make([]Point, 0, numberOfPts)

	latDelta := (dest.Latitude - source.Latitude) / float64(numberOfPts)
	longDelta := (dest.Longitude - source.Longitude) / float64(numberOfPts)

	var lat float64 = source.Latitude
	var long float64 = source.Longitude

	for i := 0; i < numberOfPts; i++ {
		lat += latDelta
		long += longDelta
		pts = append(pts, Point{lat, long})
	}

	return pts
}

func main() {
	client := setupClient()
	ertTeamCount := 5
	ertTeams := generateErtTeams(ertTeamCount)

	subscribe(client)

	for i := range ertTeams {
		fmt.Println(ertTeams[i].ID)
		go updatePosition(ertTeams[i], client)
	}

	publish(client)

	select {}
}

func generateErtTeams(ertTeamCount int) []ERT {
	ertTeams := make([]ERT, 0, ertTeamCount)

	for i := 0; i < ertTeamCount; i++ {
		name := fmt.Sprintf("ERT %d", i)
		fireTruck := ERTMember{
			ERTType:         FIRE_TRUCK,
			currentLocation: randomPt(),
			staticRoute:     genStaticRoute(3),
		}
		fireTruck.destination = fireTruck.staticRoute[0]
		fireTruck.destinationRoute = generateDestinationRoute(fireTruck.currentLocation, fireTruck.destination, 5)

		ambulance := ERTMember{
			ERTType:         AMBULANCE,
			currentLocation: randomPt(),
			staticRoute:     genStaticRoute(3),
		}
		ambulance.destination = ambulance.staticRoute[0]
		ambulance.destinationRoute = generateDestinationRoute(ambulance.currentLocation, ambulance.destination, 6)

		patrolCar := ERTMember{
			ERTType:         PATROL_CAR,
			currentLocation: randomPt(),
			staticRoute:     genStaticRoute(8),
		}
		patrolCar.destination = patrolCar.staticRoute[0]
		patrolCar.destinationRoute = generateDestinationRoute(patrolCar.currentLocation, patrolCar.destination, 9)

		team := ERT{
			ID:        name,
			fireTruck: fireTruck,
			ambulance: ambulance,
			patrolCar: patrolCar,
		}
		ertTeams = append(ertTeams, team)
	}
	return ertTeams
}

func updatePosition(eRT ERT, client mqtt.Client) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		eRT.fireTruck.currentLocation = eRT.fireTruck.destinationRoute[eRT.fireTruck.destRtIdx]
		if eRT.fireTruck.destRtIdx < len(eRT.fireTruck.destinationRoute)-1 {
			eRT.fireTruck.destRtIdx++
		}

		eRT.ambulance.currentLocation = eRT.ambulance.destinationRoute[eRT.ambulance.destRtIdx]
		if eRT.ambulance.destRtIdx < len(eRT.ambulance.destinationRoute)-1 {
			eRT.ambulance.destRtIdx++
		}

		eRT.patrolCar.currentLocation = eRT.patrolCar.destinationRoute[eRT.patrolCar.destRtIdx]
		if eRT.patrolCar.destRtIdx < len(eRT.patrolCar.destinationRoute)-1 {
			eRT.patrolCar.destRtIdx++
		}

		payload := LocationPayload{
			Id:        eRT.ID,
			FireTruck: eRT.fireTruck.currentLocation,
			Ambulance: eRT.ambulance.currentLocation,
			PatrolCar: eRT.patrolCar.currentLocation,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			fmt.Errorf("Error: %v\n", err)
		}

		client.Publish(ertChannel, 0, false, payloadBytes)
	}
}

func setupClient() mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID("Simulator")
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	return client
}
