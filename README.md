# Project Scope: EVE Online Route Planning Discord Bot

## Summary

This project is to create a Go-based Discord bot that provides EVE Online pilots with the shortest travel routes. The bot's primary function is to replicate and modernize the logic of the "Short Circuit" application by interacting with a self-hosted Tripwire instance to incorporate real-time wormhole data into its calculations.

***

## Core Functionality

### 1. Tripwire Data Integration

The bot will access tripwire and grab the existing maps.
*~~ Short circuit web scrapes, do I do the same? Or try get connection to the DB?~~
* Parsing this data to understand the origin, destination, and characteristics (e.g., size, stability) of each wormhole.
* Do I creare a generated route or integrate options?.

### 2. Shortest Route Calculation

Using the combined map of static stargate data and live Tripwire wormhole data, the bot will:
* Calculate the most efficient (shortest) jump path between a user-specified origin and destination system.
* Present the resulting route to the user in a clear, step-by-step format within Discord.

***

### 3. Notes on Complications/Developments
* Bot needs to be aware of lifespan of WH in question
* ESI converter required
* User testing required

### 4. User Interaction

* Do I integrate the thera bot idea?
* Integrate API for eve scout?

***


## Primary User Command

The main interaction with the bot will be through a simple command:

* `/route [from] [to]`
    * **Description**: Calculates the shortest route, considering all available stargates and active Tripwire wormholes.
    * **Example**: `/route from: Jita to: Hek`

***

## Key Technologies

* **Language**: Go
* **Discord Integration**: DiscordGo library
