#!/bin/bash

# External Transaction Generation Script
# This script sends a transaction to the appchain that generates an external transaction to Sepolia
# Your teammate can use this to test the explorer's external transaction handling

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# API endpoint
API_URL="${API_URL:-http://localhost:8080/rpc}"
CONTENT_TYPE="Content-Type: application/json"

# Counter for request IDs
REQUEST_ID=1

# Retry settings for transaction status polling
STATUS_MAX_ATTEMPTS=10
STATUS_SLEEP=2

# Function to print colored messages
print_header() {
    echo -e "\n${BLUE}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${YELLOW}$1${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════════════════════${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

# Function to generate unique transaction hash
generate_tx_hash() {
    payload="$(date +%s%N)$RANDOM"

    if command -v sha256sum >/dev/null 2>&1; then
        hash=$(printf "%s" "$payload" | sha256sum | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        hash=$(printf "%s" "$payload" | shasum -a 256 | awk '{print $1}')
    elif command -v openssl >/dev/null 2>&1; then
        hash=$(printf "%s" "$payload" | openssl dgst -sha256 | awk '{print $NF}')
    else
        hash=$(printf "%s" "$payload" | xxd -p -c 256 | tr -d '\n')
    fi

    hash=$(echo -n "$hash" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
    echo "0x$hash"
}

# Function to send a transaction that generates an external transaction
send_ext_txn() {
    local sender=$1
    local receiver=$2
    local value=$3
    local token=$4

    local tx_hash=$(generate_tx_hash)

    print_info "Generating external transaction..."
    echo -e "${BLUE}Details:${NC}"
    echo "  Sender:   $sender"
    echo "  Receiver: $receiver"
    echo "  Value:    $value"
    echo "  Token:    $token"
    echo "  TX Hash:  $tx_hash"
    echo ""

    # Build request with generate_ext_txn flag set to true
    local params="{\"sender\":\"$sender\",\"receiver\":\"$receiver\",\"value\":$value,\"token\":\"$token\",\"hash\":\"$tx_hash\",\"generate_ext_txn\":true}"
    local request="{\"jsonrpc\":\"2.0\",\"method\":\"sendTransaction\",\"params\":[$params],\"id\":$REQUEST_ID}"

    echo -e "${BLUE}Request:${NC}"
    echo "$request" | jq '.' 2>/dev/null || echo "$request"

    # Send transaction
    local response=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$request")
    local curl_exit_code=$?

    echo -e "${GREEN}Response:${NC}"
    echo "$response" | jq '.' 2>/dev/null || echo "$response"

    # Check if curl failed
    if [ $curl_exit_code -ne 0 ]; then
        print_error "Request failed - server not responding (curl exit code: $curl_exit_code)"
        return 1
    fi

    # Check if response contains error
    if echo "$response" | grep -q '"error"'; then
        print_error "Transaction failed"
        return 1
    fi

    print_success "Transaction sent successfully!"

    # Wait and check status
    print_info "Waiting for transaction to be processed..."
    sleep $STATUS_SLEEP

    # Check transaction status
    check_tx_status "$tx_hash"

    return 0
}

# Function to check transaction status
check_tx_status() {
    local tx_hash=$1
    print_info "Checking status for transaction: $tx_hash"

    local params="\"$tx_hash\""
    local status=""
    local attempt=0
    local max_attempts=${STATUS_MAX_ATTEMPTS}

    while [ $attempt -lt $max_attempts ]; do
        attempt=$((attempt + 1))

        local request="{\"jsonrpc\":\"2.0\",\"method\":\"getTransactionStatus\",\"params\":[${params}],\"id\":${REQUEST_ID}}"
        REQUEST_ID=$((REQUEST_ID + 1))

        echo -e "${BLUE}Status Check (attempt $attempt/${max_attempts}):${NC}"

        local response=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$request")
        local curl_exit_code=$?

        echo "$response" | jq '.' 2>/dev/null || echo "$response"

        if [ $curl_exit_code -ne 0 ]; then
            print_error "Status check failed - server not responding"
            return 1
        fi

        # Extract status
        if command -v jq >/dev/null 2>&1; then
            status=$(echo "$response" | jq -r '.result // empty' 2>/dev/null)
        else
            status=$(echo "$response" | grep -oE '"result"\s*:\s*"[^"]*"' | sed -E 's/.*"result"\s*:\s*"(.*)".*/\1/' | head -n1)
        fi

        status=$(echo -n "$status" | tr -d '\r\n' | tr '[:upper:]' '[:lower:]')

        if [ "$status" == "processed" ]; then
            print_success "Transaction processed successfully!"
            print_success "An external transaction to Sepolia has been generated!"
            return 0
        elif [ "$status" == "failed" ]; then
            print_error "Transaction failed"
            return 1
        fi

        if [ $attempt -lt $max_attempts ]; then
            print_info "Status: $status — sleeping ${STATUS_SLEEP}s and retrying"
            sleep ${STATUS_SLEEP}
        fi
    done

    print_error "Could not determine final status after $max_attempts attempts (last status: $status)"
    return 1
}

# Main function
main() {
    print_header "EXTERNAL TRANSACTION GENERATOR"

    # Parse command line arguments
    SENDER="${1:-alice}"
    RECEIVER="${2:-bob}"
    VALUE="${3:-100}"
    TOKEN="${4:-USDT}"

    echo ""
    echo -e "${YELLOW}Configuration:${NC}"
    echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}API Endpoint:${NC} $API_URL"
    echo -e "${BLUE}Sender:${NC}       $SENDER"
    echo -e "${BLUE}Receiver:${NC}     $RECEIVER"
    echo -e "${BLUE}Value:${NC}        $VALUE"
    echo -e "${BLUE}Token:${NC}        $TOKEN"
    echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Check if jq is installed
    if ! command -v jq &> /dev/null; then
        print_error "jq is not installed. Install it for better JSON formatting:"
        echo "  brew install jq  (on macOS)"
        echo "  apt-get install jq  (on Ubuntu/Debian)"
        echo ""
        print_info "Continuing without JSON formatting..."
    fi

    # Check if API is running
    print_info "Checking if API is running at $API_URL..."
    if ! curl -s -f -X POST "$API_URL" -H "$CONTENT_TYPE" -d '{"jsonrpc":"2.0","method":"getTransactionStatus","params":["0x0"],"id":0}' > /dev/null 2>&1; then
        print_error "API is not responding at $API_URL"
        print_error "Please start the appchain server first with: docker-compose up"
        exit 1
    fi
    print_success "API is running!"

    echo ""

    # Send transaction that generates external transaction
    send_ext_txn "$SENDER" "$RECEIVER" "$VALUE" "$TOKEN"

    if [ $? -eq 0 ]; then
        echo ""
        print_header "SUCCESS"
        print_success "External transaction generated successfully!"
        print_info "The transaction should appear on Sepolia testnet soon"
        echo ""
    else
        echo ""
        print_header "FAILED"
        print_error "Failed to generate external transaction"
        exit 1
    fi
}

# Show usage if --help is provided
if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    echo "Usage: $0 [SENDER] [RECEIVER] [VALUE] [TOKEN]"
    echo ""
    echo "Arguments:"
    echo "  SENDER   - Sender account name (default: alice)"
    echo "  RECEIVER - Receiver account name (default: bob)"
    echo "  VALUE    - Amount to transfer (default: 100)"
    echo "  TOKEN    - Token symbol (default: USDT)"
    echo ""
    echo "Examples:"
    echo "  $0                          # Use defaults (alice -> bob, 100 USDT)"
    echo "  $0 alice charlie 500 BTC    # alice -> charlie, 500 BTC"
    echo ""
    echo "Environment Variables:"
    echo "  API_URL - Override API endpoint (default: http://localhost:8080/rpc)"
    echo ""
    exit 0
fi

# Run main function
main "$@"
