import { Mistral } from "@mistralai/mistralai";
import { setTimeout } from "timers/promises";
import dotenv from 'dotenv';

// Load environment variables
dotenv.config();

const mistralApiKey = process.env.MISTRAL_API_KEY;
const model = 'open-mistral-nemo'

// Validate environment variables
if (!mistralApiKey) {
	console.error('Error: cannot run agent, MISTRAL_API_KEY must be set in the environment variables');
	process.exit(1);
}

// const sampleUserAction = 'I am interested in this product when can I receive it?'
// const customerId = '1111'
// const productDescription = 'Peanuts collectible Swatch with red straps.'

const MAX_RETRIES = 5;
const INITIAL_BACKOFF = 1000; // 1 second

const mistral = new Mistral({ apiKey: mistralApiKey });

const namesToFunctions = {
	availableTrafficData: availableTrafficData,
	availableVehicles: availableVehicles,
};

const tools = [
	{
		"type": "function",
		"function": {
			"name": "availableTrafficData",
			"description": "Get the most recent traffic data between two a start address and a destination address",
			"parameters": {
				"type": "object",
				"properties": {
					"startAddress": {
						"type": "string",
						"description": "The start address.",
					},
					"destinationAddress": {
						"type": "string",
						"description": "The destination address.",
					}
				},
				"required": [ "startAddress", "destinationAddress" ],
			},
		},
	},
	{
		"type": "function",
		"function": {
			"name": "availableVehicles",
			"description": "Returns the currently available delivery vehicles to used for future deliveries",
			"parameters": {},
		},
	}
]

const systemPrompt = 'You are a delivery manager with 20 years experience working from distribution centers ' +
	'for the largest ecommerce companies. You understand how to calculate how long a delivery from ' +
	'a warehouse to a customer would take. Hence you are very capable in creating accurate delivery ' +
	'estimates and helping with delivery enquiries.';

function getCurrentTimestamp() {
	const now = new Date();
	return now.toISOString().split('.')[0] + 'Z';
}

function availableTrafficData({ startAddress, destinationAddress }) {
	return `{
  "route_summary": {
    "start_address": "${startAddress}",
    "end_address": "${destinationAddress}",
    "total_distance_km": 630.97,
    "estimated_duration_hours": 9.5,
    "timestamp": "${getCurrentTimestamp()}"
  },
  "route_segments": [
    {
      "segment_id": "A90-ABD-DND",
      "name": "A90 Aberdeen to Dundee",
      "length_km": 108,
      "current_average_speed_kph": 95,
      "normal_average_speed_kph": 100,
      "congestion_level": "light",
      "incidents": []
    },
    {
      "segment_id": "M90-PER-EDI",
      "name": "M90 Perth to Edinburgh",
      "length_km": 45,
      "current_average_speed_kph": 110,
      "normal_average_speed_kph": 110,
      "congestion_level": "none",
      "incidents": [
        {
          "type": "roadworks",
          "location": "Junction 3",
          "description": "Lane closure for resurfacing",
          "delay_minutes": 10
        }
      ]
    },
    {
      "segment_id": "A1-NCL-YRK",
      "name": "A1 Newcastle to York",
      "length_km": 140,
      "current_average_speed_kph": 100,
      "normal_average_speed_kph": 110,
      "congestion_level": "moderate",
      "incidents": [
        {
          "type": "accident",
          "location": "Near Darlington",
          "description": "Multi-vehicle collision",
          "delay_minutes": 25
        }
      ]
    },
    {
      "segment_id": "M1-LEE-LON",
      "name": "M1 Leeds to London",
      "length_km": 310,
      "current_average_speed_kph": 105,
      "normal_average_speed_kph": 110,
      "congestion_level": "light",
      "incidents": []
    },
    {
      "segment_id": "A12-LON-E2",
      "name": "A12 to East London",
      "length_km": 15,
      "current_average_speed_kph": 30,
      "normal_average_speed_kph": 50,
      "congestion_level": "heavy",
      "incidents": [
        {
          "type": "congestion",
          "location": "Approach to Blackwall Tunnel",
          "description": "Heavy traffic due to rush hour",
          "delay_minutes": 20
        }
      ]
    }
  ],
  "weather_conditions": [
    {
      "location": "Aberdeen",
      "condition": "light_rain",
      "temperature_celsius": 12
    },
    {
      "location": "Edinburgh",
      "condition": "cloudy",
      "temperature_celsius": 14
    },
    {
      "location": "Newcastle",
      "condition": "clear",
      "temperature_celsius": 16
    },
    {
      "location": "London",
      "condition": "partly_cloudy",
      "temperature_celsius": 18
    }
  ]
}`
}

function availableVehicles() {
	return `[
    {
        "vehicle": {
            "make": "AUDI",
            "model": "A6 RS6 AVANT TFSI QUATTRO",
            "year": 2024,
            "trim": "Prestige",
            "body_type": "Wagon",
            "fuel_type": "Petrol",
            "transmission": {
                "type": "DSG",
                "gears": 8
            },
            "engine": {
                "cylinders": "V8",
                "displacement_liters": 4,
                "horsepower": 591,
                "engine_code": "DJPB"
            },
            "dimensions": {
                "length_inches": 196.7,
                "width_inches": 76.8,
                "height_inches": 58.6,
                "wheelbase_inches": 115.3
            },
            "weight": {
                "curb_weight_lbs": 4740,
                "gross_weight_lbs": 5820
            },
            "performance": {
                "top_speed_mph": 155,
                "acceleration_0_60_mph_seconds": 3.5
            },
            "fuel_economy": {
                "city_mpg": 15,
                "highway_mpg": 22,
                "combined_mpg": 17
            },
            "identifiers": {
                "vin": "WAULFAF27KN123456",
                "license_plate": "AB12 CDE"
            },
            "color": {
                "exterior": "Black",
                "interior": "Black"
            },
            "features": [
                "All-Wheel Drive",
                "Adaptive Suspension",
                "Bang & Olufsen Sound System",
                "Head-Up Display"
            ],
            "maintenance": {
                "last_service_date": "2024-05-15",
                "odometer_reading_miles": 12500,
                "next_service_due_miles": 20000
            },
            "market_data": {
                "msrp_usd": 118500,
                "invoice_price_usd": 111390
            }
        }
      }
]`
}

function createPrompt(opts) {
	return `Please provide a "Best Case" and "Worst Case" date estimates for delivering a package, given:
PRODUCT AVAILABILITY: ${opts?.productAvailability}
WAREHOUSE ADDRESS: ${opts?.warehouseAddress}
CUSTOMER ADDRESS: ${opts?.customerAddress}

Ensure you consider the following conditions:
- Current traffic conditions
- Reported incidents and delays
- Weather conditions
- Vehicle performance capabilities
- Potential for additional unforeseen delays

This is specifically for customer with ID: ${opts?.customerId} and customer name: ${opts?.customerName}

The requested product details are: ${opts?.productDescription}

Return the required information in JSON format.

USE THIS SCHEMA FOR THE FINAL ANSWER:
{
	"customer": {
		"customerId": "the customer id",
		"customerName": "the customer name"
	}
	"products": [
		"productDescription": "the product description"
	],
  "delivery_estimates": {
    "route_summary": {
      "start_address": "the start address",
      "end_address": "the end address",
      "total_distance_km": expected distance as number,
      "base_estimated_duration_hours": expected duration as decimal value, e.g. 7.5,
    },
    "estimates": [
      {
        "scenario": "Best case",
        "estimated_duration_hours": expected duration as decimal value, e.g. 7.5,
        "departure_time": "departure time as a timestamp, e.g. 2024-10-02T11:00:00Z",
        "estimated_arrival_time": "estimated time of arrival as a timestamp, e.g. 2024-10-02T21:15:00Z",
        "confidence_level": "how confident you are. one of: low, moderate or high"
      },
      {
        "scenario": "Worst case",
        "estimated_duration_hours": expected duration as decimal value, e.g. 7.5,
        "departure_time": "departure time as a timestamp, e.g. 2024-10-02T11:00:00Z",
        "estimated_arrival_time": "estimated time of arrival as a timestamp, e.g. 2024-10-02T21:15:00Z",
        "confidence_level": "how confident you are. one of: low, moderate or high"
      }
    ],
    "factors_considered": [ "considered traffic conditions" ] ]
  }
}
`;
}

function extractAndParseJson(input) {
	// Extract the JSON content
	const jsonMatch = input.match(/```json\n([\s\S]*?)\n```/);
	
	if (!jsonMatch) {
		throw new Error("No JSON content found between ```json``` tags");
	}
	
	const jsonString = jsonMatch[1];
	
	// Parse the JSON
	try {
		return JSON.parse(jsonString);
	} catch (error) {
		throw new Error(`Failed to parse JSON: ${error.message}`);
	}
}

async function retryWithBackoff(fn, retries = 0) {
	try {
		return await fn();
	} catch (error) {
		if (error.statusCode === 429 && retries < MAX_RETRIES) {
			const backoff = INITIAL_BACKOFF * Math.pow(2, retries);
			console.error(`Rate limit hit. Retrying in ${backoff}ms...`);
			await setTimeout(backoff);
			return retryWithBackoff(fn, retries + 1);
		}
		throw error;
	}
}

export async function runAgent(opts) {
	const prompt = createPrompt(opts);
	
	const messages = [
		{ role: 'system', content: systemPrompt },
		{ role: 'user', content: prompt }
	];
	
	let response = await retryWithBackoff(() =>
		mistral.chat.complete({
			model: model,
			messages: messages,
			tools: tools,
			toolChoice: "any"
		})
	);
	
	messages.push(response.choices[0].message);
	const toolCalls = response.choices[0].message.toolCalls;
	
	for (const toolCall of toolCalls) {
		const functionName = toolCall.function.name;
		const functionParams = JSON.parse(toolCall.function.arguments);
		const functionResult = namesToFunctions[functionName](functionParams);
		
		messages.push({
			role: "tool",
			name: functionName,
			content: functionResult,
			tool_call_id: toolCall.id,
		});
	}
	
	response = await retryWithBackoff(() =>
		mistral.chat.complete({
			model: model,
			messages: messages,
			response_format: { type: 'json_object' },
		})
	);
	
	const result = response.choices[0].message.content;
	return extractAndParseJson(result);
}
