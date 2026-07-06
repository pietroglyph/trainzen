package main

import (
	"container/list"
	"fmt"
	"math"
)

type RouteAndStops struct {
	route Route
	stops []Stop
}

type RouteAndNumStops struct {
	route    Route
	numStops int
}
type StopStatistics struct {
	MinRoute RouteAndNumStops
	MaxRoute RouteAndNumStops
}

type routesAndStopName struct {
	// Part of the node information (along with the ID stored as a key in the nodes map)
	stopName string
	// Edges connected to this node
	routes []Route
}

// A hypergraph with stops as nodes and routes as hyperedges
type StopGraph struct {
	// Maps stops IDs to routes; includes the long stop name in the value instead of the key so we only compare by ID (XXX: defending against an edge case that doesn't exist here)
	stopToRoutes map[string]routesAndStopName
	// Maps each route ID to every stop it connects
	routeToStops map[string][]Stop
}

// Basically the simplest possible setup code, which gives a stop hypergraph and
// has amortized time complexity O(∑ᵣ sᵣ), where sᵣ := number of stops on route r.
// The alternative where we build a route graph requires iterating over the
// routes for every stop and is, I think, O(∑ᵣ sᵣ + ∑ₛ rₛ²), where
// rₛ := number of routes connecting to stop s.
func BuildStopGraph(routeStops []RouteAndStops) (StopGraph, StopStatistics) {
	numRoutes := len(routeStops)
	graph := StopGraph{
		stopToRoutes: make(map[string]routesAndStopName, numRoutes*STOPS_PER_ROUTE_GENEROUS_GUESS),
		routeToStops: make(map[string][]Stop, numRoutes),
	}

	minStops := RouteAndNumStops{numStops: math.MaxInt}
	maxStops := RouteAndNumStops{numStops: math.MinInt}
	var uniqueStops []Stop
	for _, rs := range routeStops {
		numStops := len(rs.stops)

		if numStops <= minStops.numStops {
			minStops = RouteAndNumStops{rs.route, numStops}
		}
		if numStops >= maxStops.numStops {
			maxStops = RouteAndNumStops{rs.route, numStops}
		}

		graph.routeToStops[rs.route.Id] = rs.stops
		for _, stop := range rs.stops {
			node, ok := graph.stopToRoutes[stop.ID]
			if !ok {
				node.routes = make([]Route, 0, numRoutes) // XXX: premature optimization?
				node.stopName = stop.Name()
				uniqueStops = append(uniqueStops, stop)
			}

			node.routes = append(node.routes, rs.route)
			graph.stopToRoutes[stop.ID] = node
		}
	}

	return graph, StopStatistics{minStops, maxStops}
}

// BFS-ish over the stop hypergraph; this is tightly quadratic in the total number
// of stops returned by the API, i.e., Θ(∑ᵣ sᵣ²), where sᵣ := number of stops on
// route r. Normal BFS on the route graph is naturally O(num routes + num transfers)
// (the two summands are the nodes and edges, respectively).
func (g StopGraph) FindRoute(startID string, endID string) ([]string, error) {
	if _, ok := g.stopToRoutes[startID]; !ok {
		return nil, fmt.Errorf("'%s' is not a valid stop ID", startID)
	}
	if _, ok := g.stopToRoutes[endID]; !ok {
		return nil, fmt.Errorf("'%s' is not a valid stop ID", endID)
	}

	type bfsState struct {
		// Stop ID of the stop connected to this one while searching
		parentID string
		// The name of the edge color that connected us to this node from the parent node
		viaRouteName string
	}
	// Keyed by stop IDs
	state := map[string]bfsState{
		endID: {
			parentID:     endID,                                  // We search from the end since we traverse the connecting routes in reverse
			viaRouteName: g.stopToRoutes[endID].routes[0].Name(), // We "board" an arbitrary route from this stop
		},
	}

	queue := list.New()
	queue.PushBack(endID)
	for elem := queue.Front(); elem != nil; elem = elem.Next() {
		stopID := elem.Value.(string)

		// Again, we are searching *in reverse*
		if stopID == startID {
			var viaRouteNames []string
			for {
				// NB: if endID == startID then we will append nothing to the via routes, since we don't have to take any of them
				if stopID == endID {
					break
				}

				s := state[stopID]
				viaRouteNames = append(viaRouteNames, s.viaRouteName)
				stopID = s.parentID
			}

			return viaRouteNames, nil
		}

		node := g.stopToRoutes[stopID]
		// We iterate over all connecting stops by iterating over all stops that connect to each connecting route
		for _, route := range node.routes {
			for _, connectingStop := range g.routeToStops[route.Id] {
				if _, explored := state[connectingStop.ID]; explored {
					continue
				}

				state[connectingStop.ID] = bfsState{
					parentID:     stopID,
					viaRouteName: route.Name(),
				}

				queue.PushBack(connectingStop.ID)
			}
		}
	}

	return nil, fmt.Errorf("no route found")
}

// Intended to paper over the non-obvious way that the stop name and ID are split across Graph.stopToRoutes
func (g StopGraph) ForEachUniqueStop(callback func(Stop, []Route)) {
	for stopID, stopNameAndRoute := range g.stopToRoutes {
		callback(Stop{struct{ Name string }{stopNameAndRoute.stopName}, stopID}, stopNameAndRoute.routes)
	}
}
