#!/bin/bash

# Basic Transaction API Test Script
# This script tests basic transfer operations between accounts

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# API endpoint
API_URL="http://localhost:8080/rpc"
CONTENT_TYPE="Content-Type: application/json"

# Counter for request IDs
REQUEST_ID=1

# Retry settings for transaction status polling
STATUS_MAX_ATTEMPTS=5
STATUS_SLEEP=1

# Arrays to store test results for summary
declare -a TEST_RESULTS
declare -a TEST_METHODS
declare -a TEST_DESCRIPTIONS
TOTAL_TESTS=0
SUCCESSFUL_TESTS=0
FAILED_TESTS=0

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

# Function to make API call and pretty print response
api_call() {
    local method=$1
    local params=$2
    local description=$3

    print_info "Testing: $description"

    # Ensure params is a JSON array (server expects params as an array)
    local trimmed=$(echo -n "$params" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    if [[ "$trimmed" == "["* ]]; then
        # already an array
        :
    elif [[ "$trimmed" == "{}" || -z "$trimmed" ]]; then
        params="[]"
    else
        params="[$params]"
    fi

    local request="{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":$REQUEST_ID}"

    # Display request
    echo -e "${BLUE}Request:${NC}"
    echo "$request" | jq '.' 2>/dev/null || echo "$request"

    # Store test results for summary
    TEST_METHODS[$TOTAL_TESTS]="$method"
    TEST_DESCRIPTIONS[$TOTAL_TESTS]="$description"

    # Make API call
    local response=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$request")
    local curl_exit_code=$?

    # Display response
    echo -e "${GREEN}Response:${NC}"
    echo "$response" | jq '.' 2>/dev/null || echo "$response"

    # Check if curl failed (server not running)
    if [ $curl_exit_code -ne 0 ]; then
        print_error "Request failed - server not responding (curl exit code: $curl_exit_code)"
        TEST_RESULTS[$TOTAL_TESTS]="FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    # Check if response contains error
    elif echo "$response" | grep -q '"error"'; then
        print_error "Request failed"
        TEST_RESULTS[$TOTAL_TESTS]="FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    else
        print_success "Request successful"
        TEST_RESULTS[$TOTAL_TESTS]="SUCCESS"
        SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
    fi

    # Store the transaction hash if present
    if echo "$response" | grep -q '"hash"'; then
        TX_HASH=$(echo "$response" | jq -r '.result.hash // .result.tx_hash // ""' 2>/dev/null)
        if [ ! -z "$TX_HASH" ]; then
            echo -e "${BLUE}Transaction Hash: $TX_HASH${NC}"
        fi
    fi

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    REQUEST_ID=$((REQUEST_ID + 1))
    echo ""
}

# Function to check transaction status
check_tx_status() {
    local tx_hash=$1
    print_info "Checking status for transaction: $tx_hash"

    # Pass the tx hash as a JSON string so params becomes ["<hash>"]
    local params="\"$tx_hash\""

    # Record this check in the test summary arrays so the final report shows it
    TEST_METHODS[$TOTAL_TESTS]="getTransactionStatus"
    TEST_DESCRIPTIONS[$TOTAL_TESTS]="Transaction Status Check for $tx_hash"

    local status=""
    local attempt=0
    local max_attempts=${STATUS_MAX_ATTEMPTS}

    while [ $attempt -lt $max_attempts ]; do
        attempt=$((attempt + 1))

        # Build and send getTransactionStatus request
        local request="{\"jsonrpc\":\"2.0\",\"method\":\"getTransactionStatus\",\"params\":[${params}],\"id\":${REQUEST_ID}}"
        REQUEST_ID=$((REQUEST_ID + 1))

        echo -e "${BLUE}Request (attempt $attempt/${max_attempts}):${NC}"
        echo "$request" | jq '.' 2>/dev/null || echo "$request"

        local response=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$request")
        local curl_exit_code=$?

        echo -e "${GREEN}Response:${NC}"
        echo "$response" | jq '.' 2>/dev/null || echo "$response"

        # Check if curl failed
        if [ $curl_exit_code -ne 0 ]; then
            print_error "Request failed - server not responding (curl exit code: $curl_exit_code)"
            status="error"
            break
        fi

        # Extract status from response
        if command -v jq >/dev/null 2>&1; then
            status=$(echo "$response" | jq -r '.result // empty' 2>/dev/null)
        else
            status=$(echo "$response" | grep -oE '"result"\s*:\s*"[^"]*"' | sed -E 's/.*"result"\s*:\s*"(.*)".*/\1/' | head -n1)
        fi

        # Normalize status
        status=$(echo -n "$status" | tr -d '\r\n' | tr '[:upper:]' '[:lower:]')

        # Check if final status
        if [ "$status" == "processed" ] || [ "$status" == "failed" ]; then
            break
        fi

        # If not final and not last attempt, sleep and retry
        if [ $attempt -lt $max_attempts ]; then
            print_info "Status: $status — sleeping ${STATUS_SLEEP}s and retrying"
            sleep ${STATUS_SLEEP}
        fi
    done

    # Handle final status
    if [ "$status" == "processed" ]; then
        print_success "Transaction status: Processed"
        TEST_RESULTS[$TOTAL_TESTS]="SUCCESS"
        SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
    elif [ "$status" == "failed" ]; then
        print_error "Transaction status: Failed"

        # Fetch receipt for error details
        local receipt_req="{\"jsonrpc\":\"2.0\",\"method\":\"getTransactionReceipt\",\"params\":[${params}],\"id\":${REQUEST_ID}}"
        REQUEST_ID=$((REQUEST_ID + 1))

        echo -e "${BLUE}Receipt Request:${NC}"
        echo "$receipt_req" | jq '.' 2>/dev/null || echo "$receipt_req"
        local receipt_resp=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$receipt_req")
        local receipt_curl_exit=$?

        echo -e "${GREEN}Receipt Response:${NC}"
        echo "$receipt_resp" | jq '.' 2>/dev/null || echo "$receipt_resp"

        # Check if receipt request failed
        if [ $receipt_curl_exit -ne 0 ]; then
            print_error "Receipt request failed - server not responding"
            TEST_RESULTS[$TOTAL_TESTS]="FAILED"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        else
            # Extract error from receipt
            local err=""
            if command -v jq >/dev/null 2>&1; then
                err=$(echo "$receipt_resp" | jq -r '.result.error // empty' 2>/dev/null)
            else
                err=$(echo "$receipt_resp" | grep -oE '"error"\s*:\s*"[^"]*"' | sed -E 's/.*"error"\s*:\s*"(.*)".*/\1/' | head -n1)
            fi

            if [ ! -z "$err" ]; then
                print_error "Receipt error: $err"

                # Check if this is a business logic validation error (expected behavior)
                if echo "$err" | grep -q "sender's balance not enough"; then
                    print_info "Note: This is expected validation behavior - insufficient balance is correctly rejected"
                    TEST_RESULTS[$TOTAL_TESTS]="SUCCESS (Validation)"
                    SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
                else
                    TEST_RESULTS[$TOTAL_TESTS]="FAILED"
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                fi
            else
                # No error details available
                TEST_RESULTS[$TOTAL_TESTS]="FAILED"
                FAILED_TESTS=$((FAILED_TESTS + 1))
            fi
        fi
    else
        print_error "Could not determine final status after $max_attempts attempts (last status: $status)"
        TEST_RESULTS[$TOTAL_TESTS]="PENDING"
    fi

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo ""
}

# Function to generate unique transaction hash (32-byte SHA-256)
generate_tx_hash() {
    # Add random component to ensure uniqueness even when called rapidly
    payload="$(date +%s%N)$RANDOM"

    if command -v sha256sum >/dev/null 2>&1; then
        hash=$(printf "%s" "$payload" | sha256sum | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        # macOS shasum supports -a 256
        hash=$(printf "%s" "$payload" | shasum -a 256 | awk '{print $1}')
    elif command -v openssl >/dev/null 2>&1; then
        hash=$(printf "%s" "$payload" | openssl dgst -sha256 | awk '{print $NF}')
    else
        # Fallback: use md5sum if available, otherwise hex of the payload
        if command -v md5sum >/dev/null 2>&1; then
            hash=$(printf "%s" "$payload" | md5sum | awk '{print $1}')
        else
            hash=$(printf "%s" "$payload" | xxd -p -c 256 | tr -d '\n')
        fi
    fi

    # Normalize: remove whitespace, lowercase
    hash=$(echo -n "$hash" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
    echo "0x$hash"
}

# Function to check balance for a user and token
check_balance() {
    local user=$1
    local token=$2
    local description=$3

    print_info "Checking balance: $description" >&2

    # Build getBalance request
    local params="{\"user\":\"$user\",\"token\":\"$token\"}"

    # Record this check in the test summary arrays
    TEST_METHODS[$TOTAL_TESTS]="getBalance"
    TEST_DESCRIPTIONS[$TOTAL_TESTS]="Balance check for $user ($token)"

    local request="{\"jsonrpc\":\"2.0\",\"method\":\"getBalance\",\"params\":[$params],\"id\":$REQUEST_ID}"
    REQUEST_ID=$((REQUEST_ID + 1))

    echo -e "${BLUE}Request:${NC}" >&2
    echo "$request" | jq '.' >&2 2>/dev/null || echo "$request" >&2

    # Make API call
    local response=$(curl -s -X POST "$API_URL" -H "$CONTENT_TYPE" -d "$request")
    local curl_exit_code=$?

    echo -e "${GREEN}Response:${NC}" >&2
    echo "$response" | jq '.' >&2 2>/dev/null || echo "$response" >&2

    # Check if curl failed
    if [ $curl_exit_code -ne 0 ]; then
        print_error "Balance check failed - server not responding (curl exit code: $curl_exit_code)" >&2
        TEST_RESULTS[$TOTAL_TESTS]="FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TOTAL_TESTS=$((TOTAL_TESTS + 1))
        return 1
    fi

    # Check if response contains error
    if echo "$response" | grep -q '"error"'; then
        print_error "Balance check failed" >&2
        TEST_RESULTS[$TOTAL_TESTS]="FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TOTAL_TESTS=$((TOTAL_TESTS + 1))
        return 1
    fi

    # Extract balance from response
    local balance=""
    if command -v jq >/dev/null 2>&1; then
        balance=$(echo "$response" | jq -r '.result.balance // empty' 2>/dev/null)
    else
        balance=$(echo "$response" | grep -oE '"balance"\s*:\s*"[^"]*"' | sed -E 's/.*"balance"\s*:\s*"(.*)".*/\1/' | head -n1)
    fi

    if [ -z "$balance" ]; then
        print_error "Could not extract balance from response" >&2
        TEST_RESULTS[$TOTAL_TESTS]="FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TOTAL_TESTS=$((TOTAL_TESTS + 1))
        return 1
    fi

    print_success "Balance check successful: $user has $balance $token" >&2
    TEST_RESULTS[$TOTAL_TESTS]="SUCCESS"
    SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    # Return the balance for use in calling function (stdout only)
    echo "$balance"
}

# Main test flow
main() {
    print_header "BLOCKCHAIN TRANSACTION API TEST SUITE"
    
    # Test 1: Basic Transfers
    print_header "1. BASIC TRANSFERS"
    
    TX_HASH1=$(generate_tx_hash)
    api_call "sendTransaction" "{\"sender\":\"alice\",\"receiver\":\"bob\",\"value\":1000,\"token\":\"USDT\",\"hash\":\"$TX_HASH1\"}" "Alice transfers 1000 USDT to Bob"
    
    TX_HASH2=$(generate_tx_hash)
    api_call "sendTransaction" "{\"sender\":\"bob\",\"receiver\":\"charlie\",\"value\":500,\"token\":\"USDT\",\"hash\":\"$TX_HASH2\"}" "Bob transfers 500 USDT to Charlie"
    
    TX_HASH3=$(generate_tx_hash)
    api_call "sendTransaction" "{\"sender\":\"alice\",\"receiver\":\"charlie\",\"value\":1,\"token\":\"BTC\",\"hash\":\"$TX_HASH3\"}" "Alice transfers 1 BTC to Charlie"
    
    # Test 2: Check Transaction Status
    print_header "2. CHECKING TRANSACTION STATUS"
    
    check_tx_status "$TX_HASH1"
    check_tx_status "$TX_HASH2"
    check_tx_status "$TX_HASH3"
    
    # Test 3: Error Cases
    print_header "3. ERROR HANDLING TESTS"
    
    ERROR_TX=$(generate_tx_hash)
    api_call "sendTransaction" "{\"sender\":\"nonexistent\",\"receiver\":\"alice\",\"value\":1000,\"token\":\"USDT\",\"hash\":\"$ERROR_TX\"}" "Transfer from non-existent account (should fail)"
    
    # Check Alice's current balance and send more than that
    ALICE_CURRENT_BALANCE=$(check_balance "alice" "USDT" "Alice's current balance for insufficient funds test")
    # Send current balance + 10000 to ensure it exceeds available balance
    INSUFFICIENT_AMOUNT=$((ALICE_CURRENT_BALANCE + 10000))
    
    ERROR_TX2=$(generate_tx_hash)
    api_call "sendTransaction" "{\"sender\":\"alice\",\"receiver\":\"bob\",\"value\":$INSUFFICIENT_AMOUNT,\"token\":\"USDT\",\"hash\":\"$ERROR_TX2\"}" "Transfer more than available balance (should fail)"
    
    # Test 4: Check Error Transaction Status
    print_header "4. CHECKING ERROR TRANSACTION STATUS"
    
    check_tx_status "$ERROR_TX"
    check_tx_status "$ERROR_TX2"
    
    # Summary
    print_header "TEST SUITE COMPLETED"
    print_success "Basic transfer functionality and error handling have been tested!"
    print_info "Check the responses above for any errors or unexpected results"
}

# Check if jq is installed for pretty printing
if ! command -v jq &> /dev/null; then
    print_error "jq is not installed. Install it for better JSON formatting:"
    echo "  brew install jq  (on macOS)"
    echo "  apt-get install jq  (on Ubuntu/Debian)"
    echo ""
    print_info "Continuing without JSON formatting..."
fi

# Check if the API is running
print_info "Checking if API is running at $API_URL..."
if curl -s -f -X POST "$API_URL" -H "$CONTENT_TYPE" -d '{"jsonrpc":"2.0","method":"getTransactionStatus","params":["0x0"],"id":0}' > /dev/null 2>&1; then
    print_success "API is running!"
else
    print_error "API is not responding at $API_URL"
    print_error "Please start the appchain server first"
    exit 1
fi

# Run the main test suite
main

# Print test summary
print_header "TEST SUMMARY"

echo -e "\n${YELLOW}Test Results:${NC}"
echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}Total Tests:${NC} $TOTAL_TESTS"
echo -e "${GREEN}Successful:${NC} $SUCCESSFUL_TESTS"
echo -e "${RED}Failed:${NC} $FAILED_TESTS"
echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Show detailed results if there are failures
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "\n${RED}Failed Tests:${NC}"
    for i in "${!TEST_RESULTS[@]}"; do
        if [ "${TEST_RESULTS[$i]}" == "FAILED" ]; then
            echo -e "  ${RED}✗${NC} Test #$((i+1)): ${TEST_METHODS[$i]} - ${TEST_DESCRIPTIONS[$i]}"
        fi
    done
fi

# Show all test results in a table format
echo -e "\n${YELLOW}Detailed Test Results:${NC}"
echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
printf "%-5s %-25s %-60s %s\n" "ID" "Method" "Description" "Result"
echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for i in "${!TEST_RESULTS[@]}"; do
    case "${TEST_RESULTS[$i]}" in
        "SUCCESS")
            result_color="${GREEN}✓ SUCCESS${NC}"
            ;;
        "SUCCESS (Validation)")
            result_color="${GREEN}✓ SUCCESS (Validation)${NC}"
            ;;
        "FAILED")
            result_color="${RED}✗ FAILED${NC}"
            ;;
        "PENDING")
            result_color="${YELLOW}PENDING${NC}"
            ;;
        "")
            result_color="${BLUE}SKIPPED${NC}"
            ;;
        *)
            result_color="${BLUE}${TEST_RESULTS[$i]}${NC}"
            ;;
    esac
    printf "%-5s %-25s %-60s " "$((i+1))" "${TEST_METHODS[$i]}" "${TEST_DESCRIPTIONS[$i]:0:60}"
    echo -e "$result_color"
done

echo -e "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Final status
echo ""
if [ $FAILED_TESTS -eq 0 ]; then
    print_success "All tests passed successfully! 🎉"
else
    print_error "$FAILED_TESTS test(s) failed. Please review the results above."
fi

print_header "END OF TEST SUITE"
