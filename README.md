# Train-related takehome

See [design.md](./design.md) and comments for design rationale.
```
$ trainzen --help
Usage: trainzen [flags] <starting station ID> <ending station ID>  -api-key string
    	MBTA API key
  -list-stop-ids
    	List all stops IDs that can be entered as starting and ending stops
  -route-types string
    	GTFS route types to include, where multiple included route types should be comma-delineated (default "0,1")
exit status 2
```
