const axios = require(`axios`);
const fs = require(`fs`).promises;

class ESIClient{
    constructor(contactInfo) {
        this.api = axios.create({
            baseURL: "https://esi.evetech.net/latest",
            timeout: 15000,
            headers: {
                'User-Agent': `Firehawk Discord Bot (${contactInfo})`,
            }
        });

        this.cache = {
            characters: new Map(),
            corporations: new Map(),
            types: new Map(),
            systems: new Map()
        };

        this.staticSystemData = {};
    }
    async fetchAndCache(id, cacheCategory, endpoint) {
        if (!id || id === 0) return "Unknown";

        const internalCache = this.cache[cacheCategory];
        if (internalCache && internalCache.has(id)) {
            return internalCache.get(id);
        }

        try {
            const response = await this.api.get(`${endpoint}/${id}/`);
            const name = response.data.name;
            
            if (internalCache) internalCache.set(id, name);
            return name;
        } catch (error) {
            console.error(`[ESI Error] Category: ${cacheCategory}, ID: ${id} - ${error.message}`);
            return "Unknown";
        }
    }

    // --- Resolved Methods ---

    async getCharacterID(name) {
        // ESI /universe/ids/ is a POST request
        try {
            const { data } = await this.api.post('/universe/ids/', [name]);
            return data.characters?.[0]?.id || null;
        } catch (error) {
            console.error(`Could not resolve ID for ${name}`);
            return null;
        }
    }

    async getCharacterName(id) {
        return this.fetchAndCache(id, 'characters', '/characters');
    }

    async getCorporationName(id) {
        return this.fetchAndCache(id, 'corporations', '/corporations');
    }

    async getTypeName(id) { // Replaces shipNames for more generic use
        return this.fetchAndCache(id, 'types', '/universe/types');
    }

    // --- System Handling ---

    async loadSystemCache(filePath) {
        try {
            const data = await fs.readFile(filePath, 'utf8');
            this.staticSystemData = JSON.parse(data);
            return true;
        } catch (err) {
            console.error("Failed to load static system data:", err.message);
            return false;
        }
    }

    findSystemByName(name) {
        if (!name) return null;
        const query = name.toLowerCase();
        return Object.values(this.staticSystemData).find(sys => 
            sys.name.toLowerCase().startsWith(query)
        ) || null;
    }

    async getRoute(originId, destinationId) {
        try {
            const { data } = await this.api.get(`/route/${originId}/${destinationId}/`);
            return data;
        } catch (error) {
            return null;
        }
    }

    getSystemDetails(id) {
        return this.staticSystemData[id] || null;
    }
}

module.exports = ESIClient;
