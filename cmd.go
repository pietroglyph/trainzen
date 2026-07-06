package main

import (
	"container/list"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
)

type Route struct {
	Attributes struct {
		LongName string `json:"long_name"`
	}
	Id string
}
type Stop struct {
	Attributes struct {
		Name string
	}
	ID string
}

type RouteAndStops struct {
	route Route
	stops []Stop
}
type StopNode struct {
	stopName string
	routes   []Route

	connectingRouteName string
	parentStopID        string
	explored            bool
}

const API_BASE_HOST = "api-v3.mbta.com"
const STOPS_PER_ROUTE_GENEROUS_GUESS = 30 * 2 // Should keep us below a 70% load factor

func main() {
	listStopIDs := flag.Bool("list-stop-ids", false, "List all stops IDs that can be entered as starting and ending stops")
	apiKey := flag.String("api-key", "", "MBTA API key")
	flag.Parse()
	args := flag.Args()
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] <starting station ID> <ending station ID>", os.Args[0])
		flag.PrintDefaults()
	}
	if len(args) < 2 {
		flag.Usage()
		os.Exit(2)
	}
	startingStopID := args[0]
	endingStopID := args[1]

	baseURL := url.URL{
		Scheme: "https",
		Host:   API_BASE_HOST,
	}
	baseQuery := url.Values{}
	if len(*apiKey) > 0 {
		baseQuery.Add("api_key", *apiKey)
	}

	routesURL := baseURL.JoinPath("/routes")
	routesQuery := maps.Clone(baseQuery)
	// Route type 0 is light rail and 1 is heavy rail
	routesQuery.Add("filter[type]", "0,1")
	routesURL.RawQuery = routesQuery.Encode()

	routesResp, err := http.Get(routesURL.String())
	if err != nil {
		log.Fatal(err)
	}

	// json.Decoder maintains an internal buffer, and reading into an array in this manner probably requires buffering the entire response, which is suspect with external data, but 1) there are a small number of subway routes and we do not expect this to change, and 2) we need to assemble the route IDs into an array for the second question
	dec := json.NewDecoder(routesResp.Body)
	var routes struct {
		Data []Route
	}
	if err := dec.Decode(&routes); err != nil {
		log.Fatal(err)
	}
	numRoutes := len(routes.Data)

	routeAndStopsChan := make(chan RouteAndStops)
	errChan := make(chan error)
	fmt.Print("(Q1) Subway routes: ")
	for _, route := range routes.Data {
		fmt.Print(route.Attributes.LongName, ", ")

		go func(route Route, baseURL url.URL, baseQuery url.Values) {
			stopsURL := baseURL.JoinPath("/stops")
			stopsQuery := baseQuery
			stopsQuery.Add("filter[route]", route.Id)
			stopsURL.RawQuery = stopsQuery.Encode()

			stopsResp, err := http.Get(stopsURL.String())
			if err != nil {
				errChan <- err
				return
			}
			if stopsResp.StatusCode != http.StatusOK {
				errChan <- fmt.Errorf("Non-ok status: '%s'", stopsResp.Status)
				return
			}

			var stops struct{ Data []Stop }
			dec := json.NewDecoder(stopsResp.Body)
			err = dec.Decode(&stops)
			if err != nil {
				errChan <- err
				return
			}
			routeAndStopsChan <- RouteAndStops{route, stops.Data}
		}(route, baseURL, maps.Clone(baseQuery))
	}
	fmt.Println()

	type RouteAndNumStops struct {
		route    Route
		numStops int
	}
	var maxStops, minStops = RouteAndNumStops{Route{}, math.MinInt}, RouteAndNumStops{Route{}, math.MaxInt}
	stopIDToNodes := make(map[string]StopNode, numRoutes*STOPS_PER_ROUTE_GENEROUS_GUESS)
	routeIDToStops := make(map[string][]Stop, numRoutes)
	if *listStopIDs {
		fmt.Print("(**) Possible stop IDs: ")
	}
	// There are exactly as many goroutines/sends on the two channels as routes
	for range routes.Data {
		select {
		case routeAndStops := <-routeAndStopsChan:
			numStops := len(routeAndStops.stops)
			routeAndNumStops := RouteAndNumStops{routeAndStops.route, numStops}
			if minStops.numStops >= numStops {
				minStops = routeAndNumStops
			}
			if maxStops.numStops <= numStops {
				maxStops = routeAndNumStops
			}

			routeIDToStops[routeAndStops.route.Id] = routeAndStops.stops
			for _, stop := range routeAndStops.stops {
				node, ok := stopIDToNodes[stop.ID]
				if !ok {
					node = StopNode{stop.Attributes.Name, make([]Route, 0, numRoutes), "", "", false}
					// Wrapped in the ok check to avoid printing duplicates
					if *listStopIDs {
						fmt.Printf("%s (%s), ", stop.ID, stop.Attributes.Name)
					}
				}
				node.routes = append(node.routes, routeAndStops.route)
				stopIDToNodes[stop.ID] = node
			}
		case err := <-errChan:
			log.Fatal(err)
		}
	}
	fmt.Printf("(Q2.1-2) %s has the most stops (%d) and %s has the fewest (%d)\n", maxStops.route.Attributes.LongName, maxStops.numStops, minStops.route.Attributes.LongName, minStops.numStops)

	fmt.Print("(Q2.3) ")
	for _, node := range stopIDToNodes {
		if len(node.routes) < 2 {
			continue
		}
		fmt.Printf("%s connects ", node.stopName)
		for _, route := range node.routes {
			fmt.Print(route.Attributes.LongName, ", ")
		}
	}
	fmt.Println()

	if _, ok := stopIDToNodes[startingStopID]; !ok {
		log.Fatal(startingStopID, " is not a valid stop ID")
	}
	if _, ok := stopIDToNodes[endingStopID]; !ok {
		log.Fatal(endingStopID, " is not a valid stop ID")
	}

	// We perform a breadth-first search and build the edges inside the search
	fmt.Printf("(Q3) %s → %s: ", stopIDToNodes[startingStopID].stopName, stopIDToNodes[endingStopID].stopName)
	queue := list.New()
	startingNode := stopIDToNodes[startingStopID]
	startingNode.explored = true
	startingNode.parentStopID = startingStopID
	startingNode.connectingRouteName = startingNode.routes[0].Attributes.LongName
	stopIDToNodes[startingStopID] = startingNode
	queue.PushBack(startingStopID)
	for currentElement := queue.Front(); currentElement != nil; currentElement = currentElement.Next() {
		stopID := currentElement.Value.(string)
		stopNode := stopIDToNodes[stopID]
		if stopID == endingStopID {
			parentStopNode := stopNode
			for {
				fmt.Print(parentStopNode.connectingRouteName, ", ")
				if parentStopNode.parentStopID == startingStopID {
					fmt.Println()
					return
				}
				parentStopNode = stopIDToNodes[parentStopNode.parentStopID]
			}
		}
		for _, route := range stopNode.routes {
			// Index into routeIDToStops to get connecting stops; for each connecting stop, check explored label, label as explored, set parent(!!), and push the stop onto queue
			for _, connectingStop := range routeIDToStops[route.Id] {
				connectingStopNode := stopIDToNodes[connectingStop.ID]
				if connectingStopNode.explored {
					continue
				}
				connectingStopNode.explored = true
				connectingStopNode.parentStopID = stopID
				connectingStopNode.connectingRouteName = route.Attributes.LongName
				stopIDToNodes[connectingStop.ID] = connectingStopNode
				queue.PushBack(connectingStop.ID)
			}
		}
	}

	log.Fatal("Could not find route")
}
