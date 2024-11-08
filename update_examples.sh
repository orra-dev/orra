#!/bin/bash

#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

# Default values
UPDATE_ENV=false
CLEAN_SVC_KEYS=false
REINSTALL_NPM=false
RUN_SERVICES=false
STOP_SERVICES=false

# Function to display usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Options:"
    echo "  -u, --update-env      Update environment variables"
    echo "  -k, --api-key KEY     Use existing API key instead of generating new one"
    echo "  -w, --webhook URL     Webhook URL to use"
    echo "  -c, --clean-svc-keys  Clean service keys"
    echo "  -i, --install-npm     Reinstall npm packages"
    echo "  -r, --run-services    Run services"
    echo "  -s, --stop-services   Stop services"
    echo "  -h, --help            Show this help message"
}

# Parse command line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -u|--update-env) UPDATE_ENV=true ;;
        -k|--api-key)
            shift
            EXTERNAL_API_KEY="$1"
            ;;
        -w|--webhook)
            shift
            WEBHOOK_URL="$1"
            ;;
        -c|--clean-svc-keys) CLEAN_SVC_KEYS=true ;;
        -i|--install-npm) REINSTALL_NPM=true ;;
        -r|--run-services) RUN_SERVICES=true ;;
        -s|--stop-services) STOP_SERVICES=true ;;
        -h|--help)
            echo "Usage: $0 [-u|--update-env] [-k|--api-key KEY] [-w|--webhook URL] [-c|--clean-svc-keys] [-i|--install-npm] [-r|--run-services] [-s|--stop-services] [-h|--help]"
            echo "  -u, --update-env      Update environment variables"
            echo "  -k, --api-key KEY     Use existing API key instead of generating new one"
            echo "  -w, --webhook URL     Webhook URL to use"
            echo "  -c, --clean-svc-keys  Clean service keys"
            echo "  -i, --install-npm     Reinstall npm packages"
            echo "  -r, --run-services    Run services"
            echo "  -s, --stop-services   Stop services"
            echo "  -h, --help            Show this help message"
            exit 0
            ;;
        *) echo "Unknown parameter passed: $1"; usage; exit 1 ;;
    esac
    shift
done

# Validate required arguments when updating env
if [ "$UPDATE_ENV" = true ] && [ -z "$WEBHOOK_URL" ]; then
    echo "Error: Webhook URL (-w|--webhook) is required when updating environment variables"
    usage
    exit 1
fi

# Directory containing the examples
EXAMPLES_DIR="./examples"

# Array of example directories
example_dirs=(
  "echo-js"
  "ecommerce-chat-js/chat-app"
  "ecommerce-chat-js/customer-svc"
  "ecommerce-chat-js/delivery-agent"
  "ecommerce-chat-js/inventory-svc"
)

# Update environment variables
if $UPDATE_ENV; then
    echo "Updating environment variables..."

    # Only call the API if no external API key is provided
    if [ -z "$EXTERNAL_API_KEY" ]; then
        echo "Registering new project..."

        # Run the curl command and capture the output
        CURL_OUTPUT=$(curl -X POST http://localhost:8005/register/project \
          -H "Content-Type: application/json" \
          -d "{\"webhook\": \"${WEBHOOK_URL}\"}" | gojq .)

        # Check if curl command was successful
        # shellcheck disable=SC2181
        if [ $? -ne 0 ]; then
            echo "Error: Failed to register project. Curl command failed."
            exit 1
        fi

        # Extract API key from the curl output
        API_KEY=$(echo "$CURL_OUTPUT" | gojq -r '.apiKey')

        # Check if API_KEY was extracted successfully
        if [ -z "$API_KEY" ]; then
            echo "Error: Failed to extract API key from the response."
            exit 1
        fi
    else
        echo "Using provided API key..."
        API_KEY="$EXTERNAL_API_KEY"
    fi

    for dir in "${example_dirs[@]}"; do
        full_dir="$EXAMPLES_DIR/$dir"
        echo "Updating environment files in $full_dir"

        # Update .env and .env.local files
        for env_file in "$full_dir/.env" "$full_dir/.env.local"; do
            if [ -f "$env_file" ]; then
                # Use a temporary file for sed operations
                temp_file=$(mktemp)
                sed -e "s/^ORRA_API_KEY=.*/ORRA_API_KEY=$API_KEY/" \
                    "$env_file" > "$temp_file"
                mv "$temp_file" "$env_file"
                echo "Updated $env_file"
            fi
        done
    done
fi

# Update environment variables
if $CLEAN_SVC_KEYS; then
    echo "Cleaning service key files..."

    for dir in "${example_dirs[@]}"; do
        full_dir="$EXAMPLES_DIR/$dir"
        echo "Attempting to remove service key in $full_dir"

        # Clean up orra-service-key.json files
        service_key_file="$full_dir/orra-service-key.json"
        if [ -f "$service_key_file" ]; then
            echo "Removing $service_key_file"
            rm "$service_key_file"
        else
          echo "No service key file to remove in $full_dir"
        fi
    done
fi

# Reinstall npm packages
if $REINSTALL_NPM; then
    echo "Reinstalling npm packages..."
    for dir in "${example_dirs[@]}"; do
        full_dir="$EXAMPLES_DIR/$dir"
        if [ -f "$full_dir/package.json" ]; then
            echo "Processing $full_dir"

            # Remove old node_modules
            rm -rf "$full_dir/node_modules"

            # Install packages
            (cd "$full_dir" && npm install)
        else
            echo "Warning: No package.json found in $full_dir"
        fi
    done
fi

# Run example services
if $RUN_SERVICES; then
    echo "Running example services..."

    # Run each service in the background
    for service in "${example_dirs[@]}"; do
        service_dir="$EXAMPLES_DIR/$service"
        if [ -d "$service_dir" ]; then
            echo "Starting $service"
            (cd "$service_dir" && npm run dev) &
        else
            echo "Warning: $service directory not found"
        fi
    done

    # Wait for all background processes
    wait
fi

# Stop example services
if $STOP_SERVICES; then
    echo "Stopping example services..."

    for service in "${example_dirs[@]}"; do
        service_name=$(basename "$service")
        pids=$(pgrep -f "node.*$service_name")
        if [ -n "$pids" ]; then
            echo "Stopping $service_name (PIDs: $pids)"
            kill "$pids"
        else
            echo "No running process found for $service_name"
        fi
    done

    # Wait a moment to allow processes to stop
    sleep 2

    # Check if any processes are still running
    for service in "${example_dirs[@]}"; do
        service_name=$(basename "$service")
        if pgrep -f "node.*$service_name" > /dev/null; then
            echo "Warning: $service_name is still running"
        else
            echo "$service_name stopped successfully"
        fi
    done
fi

echo "Script completed."
