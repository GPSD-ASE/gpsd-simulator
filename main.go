package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	groupID = "gpsd"

	dublinLatitude  = 53.350140
	dublinLongitude = -6.266155

	incidentChannel = "incident"
	ertChannel      = "location"
)

var MAP_MGMT_ENDPOINT string

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
	ID         string
	ERTType    ERTType
	location   Point
	dest       Point
	destRt     Route
	destIdx    int
	patrol     Route
	rtIdx      int
	dispatched bool
}

type MAP_MGMT_PAYLOAD struct {
	Paths []struct {
		Points struct {
			Coords [][]float64 `json:"coordinates"`
		} `json:"points"`
	} `json:"paths"`
}

var ertTeams struct {
	ftTeams  []ERT
	ambTeams []ERT
	pcTeams  []ERT
}

func messagePubHandler(topic string, msg []byte) {
	fmt.Printf("Topic: %s\tMessage: %s", topic, msg)

	switch topic {
	case ertChannel:

		var payload LocationPayload
		err := json.Unmarshal(msg, &payload)
		if err != nil {
			fmt.Println(err)
			return
		}

	case incidentChannel:
		var payload IncidentPayload
		err := json.Unmarshal(msg, &payload)
		if err != nil {
			fmt.Println(err)
			return
		}

		ftIdx := getNearestN(ertTeams.ftTeams, payload.FtCount, payload.Location)
		fmt.Println("Nearest ft: ", ftIdx)

		for _, v := range ftIdx {
			ertTeams.ftTeams[v].dest = payload.Location
			ertTeams.ftTeams[v].destIdx = 0
			ertTeams.ftTeams[v].destRt = genDestPts(ertTeams.ftTeams[v].location, payload.Location)
			ertTeams.ftTeams[v].dispatched = true
		}

		ambIdx := getNearestN(ertTeams.ambTeams, payload.AmbCount, payload.Location)
		fmt.Println("Nearest amb: ", ambIdx)

		for _, v := range ambIdx {
			ertTeams.ambTeams[v].dest = payload.Location
			ertTeams.ambTeams[v].destIdx = 0
			ertTeams.ambTeams[v].destRt = genDestPts(ertTeams.ambTeams[v].location, payload.Location)
			ertTeams.ambTeams[v].dispatched = true
		}

		pcIdx := getNearestN(ertTeams.pcTeams, payload.PcCount, payload.Location)
		fmt.Println("Nearest pc: ", pcIdx)

		for _, v := range pcIdx {
			ertTeams.pcTeams[v].dest = payload.Location
			ertTeams.pcTeams[v].destIdx = 0
			ertTeams.pcTeams[v].destRt = genDestPts(ertTeams.pcTeams[v].location, payload.Location)
			ertTeams.pcTeams[v].dispatched = true
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

		if teams[i].dispatched {
			distances[i].val = math.Inf(1)
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

func genDestPts(source, dest Point) Route {
	endpoint := fmt.Sprintf("%s?origin=%f,%f&destination=%f,%f",
		MAP_MGMT_ENDPOINT,
		source.Latitude,
		source.Longitude,
		dest.Latitude,
		dest.Longitude,
	)

	resp, err := http.Get(endpoint)
	if err != nil {
		log.Printf("Map Mgmt Error: %v", err)
		return Route{}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Map Mgmt Paylod Error: %v", err)
		return Route{}
	}

	var payload MAP_MGMT_PAYLOAD
	err = json.Unmarshal(body, &payload)
	if err != nil {
		log.Printf("Map Mgmt Unmarshall Error: %v", err)
		return Route{}
	}

	pts := make([]Point, 0)
	for _, coord := range payload.Paths[0].Points.Coords {
		pts = append(pts, Point{
			coord[0],
			coord[1],
		})
	}

	return pts

	// latDelta := (dest.Latitude - source.Latitude) / float64(numberOfPts)
	// longDelta := (dest.Longitude - source.Longitude) / float64(numberOfPts)

	// var lat float64 = source.Latitude
	// var long float64 = source.Longitude

	// for i := 0; i < numberOfPts; i++ {
	// 	lat += latDelta
	// 	long += longDelta
	// 	pts = append(pts, Point{lat, long})
	// }

	// return pts
}

func main() {
	MAP_MGMT_ENDPOINT = os.Getenv("MAP_MGMT_ENDPOINT")
	if MAP_MGMT_ENDPOINT == "" {
		log.Fatal("MAP_MGMT_ENDPOINT not set")
	}

	writer, reader := setupClient()

	go func() {
		for {
			m, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("error reading message: %v", err)
				continue
			}
			messagePubHandler(m.Topic, m.Value)
		}
	}()

	ftCount := 3
	ambCount := 5
	pcCount := 4
	ertTeams.ftTeams = genERT(FIRE_TRUCK, ftCount)
	ertTeams.ambTeams = genERT(AMBULANCE, ambCount)
	ertTeams.pcTeams = genERT(PATROL_CAR, pcCount)

	for i := range ertTeams.ftTeams {
		go updatePosition(&ertTeams.ftTeams[i], writer)
	}

	for i := range ertTeams.ambTeams {
		go updatePosition(&ertTeams.ambTeams[i], writer)
	}

	for i := range ertTeams.pcTeams {
		go updatePosition(&ertTeams.pcTeams[i], writer)
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
		fmt.Printf("error generating incident payload: %v\n", err)
	}

	time.Sleep(10 * time.Second)

	err = writer.WriteMessages(context.Background(), kafka.Message{
		Topic: incidentChannel,
		Value: payloadBytes,
	})
	if err != nil {
		fmt.Printf("error publishing incident payload: %v\n", err)
	}

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
		ert.destRt = genDestPts(ert.location, ert.dest)

		ertTeams = append(ertTeams, ert)
	}
	return ertTeams
}

func updatePosition(eRT *ERT, writer *kafka.Writer) {
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
			fmt.Printf("error generating payload: %v\n", err)
			return
		}

		err = writer.WriteMessages(context.Background(), kafka.Message{
			Topic: ertChannel,
			Value: payloadBytes,
		})
		if err != nil {
			fmt.Printf("Error publishing location payload: %v\n", err)
		}
	}
}

func updateERTPosition(member *ERT) {
	member.location = member.destRt[member.destIdx]
	member.destIdx++

	if member.location.same(member.dest) {
		if member.dispatched {
			member.dispatched = false
		}
		// fmt.Printf("%s reached\n", member.ERTType)
		member.rtIdx = (member.rtIdx + 1) % len(member.patrol)
		member.dest = member.patrol[member.rtIdx]

		member.destRt = genDestPts(member.location, member.dest)
		member.destIdx = 0
	}
}

func setupClient() (*kafka.Writer, *kafka.Reader) {
	kafkaBroker := os.Getenv("KAFKA_BROKER")

	if kafkaBroker == "" {
		log.Panicln("'KAFKA_BROKER' not set")
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaBroker},
		Balancer: &kafka.LeastBytes{},
	})

	defer func() {
		if err := writer.Close(); err != nil {
			log.Printf("failed to close kafka writer: %v", err)
		}
	}()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{kafkaBroker},
		GroupID:     groupID,
		GroupTopics: []string{ertChannel, incidentChannel},
		MinBytes:    10e3, // 10 KB
		MaxBytes:    10e6, // 10 MB
	})
	defer func() {
		if err := reader.Close(); err != nil {
			log.Printf("failed to close kafka reader: %v", err)
		}
	}()

	return writer, reader
}
