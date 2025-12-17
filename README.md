# Project Scope: Short Circuit Discord Bot

## Summary

This project is a Go-based Discord bot that provides EVE Online pilots with advanced, intel-rich travel routes. The bot modernizes the logic of route planning by combining static stargate data with live wormhole connections from multiple sources, offering customizable and tactically aware pathfinding.

***
## Core Functionality

### 1. Multi-Source Data Integration

The bot consolidates data from several key sources to build a comprehensive, real-time map of the EVE universe:
* **Static Universe Map**: Builds a baseline of all stargate connections from a local data file.
* **Tripwire Integration**: A background service periodically scrapes a Tripwire instance to get a live feed of user-maintained wormhole connections, including signature IDs and EOL data.
* **EVE-Scout Integration**: A separate background service fetches public Thera connections to ensure this critical hub is always included.
* **ESI Integration**: Fetches live data for system security status and recent player kills to provide tactical intel.

### 2. Advanced Route Calculation

Using the combined map, the bot calculates the most efficient path based on user preferences:
* **Weighted Pathfinding**: Implements Dijkstra's algorithm to find the optimal route based on user-selected preferences ("Shortest," "Safer," or "Unsafe").
* **Dynamic Intel Display**: Presents the resulting route in a clean, multi-column format within a Discord embed, showing system names, security status, recent kills, and wormhole signature information.

***
## User Interaction

The main interaction is through a single, powerful slash command:

* `/route`
    * **`start`**: The starting solar system.
    * **`end`**: The destination solar system.
    * **`exclude`**: (Optional) A comma-separated list of systems to avoid.
    * **`preference`**: (Optional) A dropdown to choose between "Shortest," "Safer," or "Unsafe" routes.

### Additional Features

* **Copy Route Button**: An interactive button that provides the user with a simple, copy-paste-friendly version of the route.
* **Background Services**: The bot uses multiple, concurrent background services to keep its wormhole and kill data up to date without impacting performance.

### Limitations and Further Dev

* **Discord** Message posting has 1024 character limit
* **API** Still to convert to its own API call which will eliminate a great deal of code.
* **Bugs** - Freezes if the route is too long - has issues if you ask the same route twice

***
## Key Technologies

* **Language**: Go
* **Discord Integration**: DiscordGo library
* **Data Sources**: EVE Online ESI, Tripwire (Web Scraper), EVE-Scout API
* **Concurrency**: Goroutines, Mutexes, and WaitGroups for efficient, thread-safe data processing.