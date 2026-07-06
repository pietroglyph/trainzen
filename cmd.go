package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const API_BASE_HOST = "api-v3.mbta.com"
const STOPS_PER_ROUTE_GENEROUS_GUESS = 30 * 2 // Should keep us below a 70% load factor

type Route struct {
	Attributes struct {
		// Just "Name" for consistency with Stop
		Name string `json:"long_name"`
	}
	Id string
}

func (r Route) Name() string {
	return r.Attributes.Name
}

type Stop struct {
	Attributes struct {
		Name string
	}
	ID string
}

func (r Stop) Name() string {
	return r.Attributes.Name
}

type Named interface {
	Name() string
}

func CollectNames[T Named](as []T) []string {
	names := make([]string, len(as))
	for i := range as {
		names[i] = as[i].Name()
	}
	return names
}

func main() {
	apiKey := flag.String("api-key", "", "MBTA API key")
	// Route type 0 is light rail and 1 is heavy rail
	routeTypes := flag.String("route-types", "0,1", "GTFS route types to include, where multiple included route types should be comma-delineated")
	listStopIDs := flag.Bool("list-stop-ids", false, "List all stops IDs that can be entered as starting and ending stops")
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
	routesQuery.Add("filter[type]", *routeTypes)
	routesURL.RawQuery = routesQuery.Encode()

	routesResp, err := http.Get(routesURL.String())
	if err != nil {
		log.Fatal(err)
	}

	// json.Decoder maintains an internal buffer, and reading into an array in this
	// manner probably requires buffering the entire response, which is suspect with
	// external data, but 1) there are a small number of subway routes and we do not
	// expect this to change, and 2) avoiding assembling the routes into an array
	// significantly complicates the messaging (likely requires a WaitGroup or
	// similar) and the processing code.
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
	fmt.Println("(Q1) 'Subway' routes:", strings.Join(CollectNames(routes.Data), ", "), ".")
	for _, route := range routes.Data {
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

	allRoutesStops := make([]RouteAndStops, 0, numRoutes)
	// There are exactly as many goroutines/sends on the two channels as routes
	for range routes.Data {
		select {
		case rs := <-routeAndStopsChan:
			allRoutesStops = append(allRoutesStops, rs)

		case err := <-errChan:
			log.Fatal(err)
		}
	}
	graph, stopStatistics := BuildStopGraph(allRoutesStops)

	fmt.Printf(
		"(Q2.1-2) The %s has the most stops (%d) and the %s has the fewest (%d).\n",
		stopStatistics.MaxRoute.route.Name(),
		stopStatistics.MaxRoute.numStops,
		stopStatistics.MinRoute.route.Name(),
		stopStatistics.MinRoute.numStops,
	)
	if *listStopIDs {
		fmt.Print("(**) Possible stop IDs: ")
		graph.ForEachUniqueStop(func(s Stop, _ []Route) {
			fmt.Printf("%s (%s), ", s.ID, s.Name())
		})
		fmt.Println()
	}

	fmt.Print("(Q2.3) ")
	graph.ForEachUniqueStop(func(s Stop, r []Route) {
		if len(r) < 2 {
			return
		}
		fmt.Print(s.Name(), " connects ", strings.Join(CollectNames(r), ", "), ". ")
	})
	fmt.Println()

	routesTaken, err := graph.FindRoute(startingStopID, endingStopID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("(Q3) %s → %s: %s.", graph.stopToRoutes[startingStopID].stopName, graph.stopToRoutes[endingStopID].stopName, strings.Join(routesTaken, ", "))
	fmt.Println()
}
