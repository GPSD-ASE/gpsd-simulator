package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"slices"
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
	Id          string  `json:"id"`
	Type        ERTType `json:"type"`
	Location    Point   `json:"location"`
	OldLocation Point   `json:"old_location"`
}

type IncidentPayload struct {
	Id       string `json:"id"`
	Location Point  `json:"location"`
	FtCount  int    `json:"ft_count"`
	AmbCount int    `json:"amb_count"`
	PcCount  int    `json:"pc_count"`
	Type     string `json:"incident_type"`
}

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (p Point) same(other Point) bool {
	return math.Abs(p.Latitude-other.Latitude) < 1e-3 && math.Abs(p.Longitude-other.Longitude) < 1e-3
}

type Route []Point

type ERTType string

const (
	FIRE_TRUCK ERTType = "Fire Truck"
	AMBULANCE  ERTType = "Ambulance"
	PATROL_CAR ERTType = "Patrol Car"
)

type ERT struct {
	ID       string
	ERTType  ERTType
	location Point
	dest     Point
	destRt   Route
	destIdx  int
	patrol   Route
	rtIdx    int
	reached  bool
}

var ertTeams struct {
	ftTeams  []ERT
	ambTeams []ERT
	pcTeams  []ERT
}

func messagePubHandler(client mqtt.Client, msg mqtt.Message) {
	// fmt.Printf("Topic: %s\n", msg.Topic())

	switch msg.Topic() {
	case ertChannel:

		var payload LocationPayload
		err := json.Unmarshal(msg.Payload(), &payload)
		if err != nil {
			fmt.Println(err)
		}

		// if payload.Type == FIRE_TRUCK {
		fmt.Printf("%s\n", msg.Payload())
		// }

	case incidentChannel:
		var payload IncidentPayload
		fmt.Printf("%s\n", msg.Payload())

		err := json.Unmarshal(msg.Payload(), &payload)
		if err != nil {
			fmt.Println(err)
			return
		}

		ftIdx := getNearestN(ertTeams.ftTeams, payload.FtCount, payload.Location)
		fmt.Println("Nearest ft: ", ftIdx)

		for _, v := range ftIdx {
			ertTeams.ftTeams[v].dest = payload.Location
			ertTeams.ftTeams[v].destIdx = 0
			ertTeams.ftTeams[v].destRt = genDestPts(ertTeams.ftTeams[v].location, payload.Location, 6)
		}

		ambIdx := getNearestN(ertTeams.ambTeams, payload.AmbCount, payload.Location)
		fmt.Println("Nearest amb: ", ambIdx)

		for _, v := range ambIdx {
			ertTeams.ambTeams[v].dest = payload.Location
			ertTeams.ambTeams[v].destIdx = 0
			ertTeams.ambTeams[v].destRt = genDestPts(ertTeams.ambTeams[v].location, payload.Location, 6)
		}

		pcIdx := getNearestN(ertTeams.pcTeams, payload.PcCount, payload.Location)
		fmt.Println("Nearest pc: ", pcIdx)

		for _, v := range pcIdx {
			ertTeams.pcTeams[v].dest = payload.Location
			ertTeams.pcTeams[v].destIdx = 0
			ertTeams.pcTeams[v].destRt = genDestPts(ertTeams.pcTeams[v].location, payload.Location, 6)
		}

	default:
	}
}

func getNearestN(teams []ERT, count int, dest Point) []int {
	type dist struct {
		idx int
		val float64
	}
	distances := make([]dist, len(teams))
	for i := range teams {
		distances[i] = dist{
			idx: i,
			val: distance(teams[i].location, dest),
		}
	}

	slices.SortFunc(distances, func(a, b dist) int {
		diff := (a.val - b.val) * 1000
		return int(diff)
	})

	fmt.Println(distances)

	indices := make([]int, count)
	for i := range indices {
		indices[i] = distances[i].idx
	}
	return indices
}

func distance(src, dest Point) float64 {
	x := src.Latitude - dest.Latitude
	x2 := x * x

	y := src.Longitude - dest.Longitude
	y2 := y * y

	return math.Sqrt(x2 + y2)
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

func genDestPts(source, dest Point, numberOfPts int) Route {
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
	ftCount := 3
	ambCount := 5
	pcCount := 4
	ertTeams.ftTeams = genERT(FIRE_TRUCK, ftCount)
	ertTeams.ambTeams = genERT(AMBULANCE, ambCount)
	ertTeams.pcTeams = genERT(PATROL_CAR, pcCount)

	subscribe(client)

	for i := range ertTeams.ftTeams {
		go updatePosition(&ertTeams.ftTeams[i], &client)
	}

	for i := range ertTeams.ambTeams {
		go updatePosition(&ertTeams.ambTeams[i], &client)
	}

	for i := range ertTeams.pcTeams {
		go updatePosition(&ertTeams.pcTeams[i], &client)
	}

	payload := IncidentPayload{
		Id: "122",
		Location: Point{
			80.0,
			-10.0,
		},
		FtCount:  2,
		AmbCount: 3,
		PcCount:  1,
		Type:     "Fire",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Errorf("Error: %v\n", err)
	}

	time.Sleep(10 * time.Second)
	client.Publish(incidentChannel, 0, false, payloadBytes)

	select {}
}

func genERT(ertType ERTType, count int) []ERT {
	ertTeams := make([]ERT, 0, count)

	for i := 0; i < count; i++ {
		ert := ERT{
			ID:       fmt.Sprintf("%s %d", ertType, i),
			location: randomPt(),
			patrol:   genStaticRoute(3),
			ERTType:  ertType,
		}

		ert.dest = ert.patrol[0]
		ert.destRt = genDestPts(ert.location, ert.dest, 1+rand.Intn(5))

		ertTeams = append(ertTeams, ert)
	}
	return ertTeams
}

func updatePosition(eRT *ERT, client *mqtt.Client) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		oldLocation := eRT.location
		updateERTPosition(eRT)

		payload := LocationPayload{
			Id:          eRT.ID,
			Type:        eRT.ERTType,
			Location:    eRT.location,
			OldLocation: oldLocation,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			fmt.Errorf("Error: %v\n", err)
		}

		(*client).Publish(ertChannel, 0, false, payloadBytes)
	}
}

func updateERTPosition(member *ERT) {
	member.location = member.destRt[member.destIdx]
	member.destIdx++

	if member.location.same(member.dest) {
		// fmt.Printf("%s reached\n", member.ERTType)
		member.rtIdx = (member.rtIdx + 1) % len(member.patrol)
		member.dest = member.patrol[member.rtIdx]

		member.destRt = genDestPts(member.location, member.dest, 1+rand.Intn(3))
		member.destIdx = 0
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
