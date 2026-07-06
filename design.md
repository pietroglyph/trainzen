# Design Diary

## Requirements and architecture
It is important to be explicit about the requirements and motivations of any new design. This is an exercise and not a real project, but there are still a few things I think are important:
 1. The program should be easy for a 3rd party to read and understand.
 2. The program's output should be easy for a 3rd party to manually verify.
 3. The program's output should be correct, in the sense that it matches the "spec" in the take-home PDF.
 4. I haven't written Go in a number of years, so I would like this project to remind me of some of its characteristics.

My usual instinct is to make a program like this into a relatively modular tool (e.g., with multiple sub-commands and a reasonable set of flags) so that it can be easily extended, composed with itself and other tools, and integration tested. I'm used to tools with this kind of behavior and interface. I think this is not the right approach, however, because:
 1. This will never be extended. Simpler code is easier to read and understand (requirement 1).
 2. Extra flags and sub-commands need to be documented for whomever is manually verifying my solution. A tool that outputs everything in one go is much easier to use during an essentially one-off manual verification (requirement 2).
 3. It seems to me (after skimming the PDF) that there are multiple opportunities for making only two or three API calls and re-using data, something that is much easier if this tool does everything in one go instead of being split into sub-commands.
A relatively monolithic interface seems appropriate, but the actual data processing code should still be modular enough to accommodate a reasonable amount of unit testing, since I would really like to be sure that I have submitted a program that is correct.

## Understanding the API
The right endpoint for question 1 is `/routes`: for heavy- and light-rail lines, the alternative `/lines` endpoint (contrary to the name) returns a strict subset of the information `/routes` returns, but cannot be filtered by route type either through the API or locally (unless one adds the query param `include=routes`).

The `/stops` endpoint is probably the right one for question 2, but its behavior is rather subtle: there are parent stops (for overall stations, with `location_type` of `1`) and these parent stops can have child "stops" for station entrances and exits, boarding locations, and other points of interest inside the station. We can filter to `location_type` of `1` only, but we cannot include the route(s) associated with each stop unless we filter to stops of a single route. Because of this, it seems we will have to call the `/stops` endpoint once for each line.

Question 3 should not need to make any API requests.

## Algorithm
Question 1 is trivial, as are question 2.1 and 2.2, provided we deserialize the JSON responses into a data structure where getting the length of the arrays of stops is in constant time. It is simpler to filter out non-subway routes on the serverside (in question 1).

Question 2.3 is less trivial, but the most obvious solution involves building a multimap from station IDs to subway lines and then iterating over the keys and values and printing every time the set of values is of size greater than one. Conveniently, the maximum size of the underlying value arrays is fixed. The space and amortized time complexity of building this multiset is O(total number of station appearances across all lines)), and the amortized time complexity of printing the transfer stations is O(number of stations).

For question 3, the most obvious solution (to me) is a breadth-first search over a graph with vertices given as subway lines with the edge set consisting of every size-two set of transfer stations between two lines. We can build the set of edges while iterating over the multimap and then do the BFS on this representation, but since we only do one query, we can avoid the time and space cost of building the line graph explicitly.
