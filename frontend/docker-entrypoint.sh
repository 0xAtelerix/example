#!/bin/sh
set -e

# Generate config.js from environment variables
cat > /usr/share/nginx/html/config.js <<EOF
const CONFIG = {
    BRIDGE_SEPOLIA: '${BRIDGE_CONTRACT_SEPOLIA}',
    BRIDGE_STAVANGER: '${BRIDGE_CONTRACT_STAVANGER}',
    POL_TOKEN_SEPOLIA: '${POL_TOKEN_SEPOLIA}',
    SEPOLIA_CHAIN_ID: 11155111,
    STAVANGER_CHAIN_ID: 50591822,
    SEPOLIA_RPC: '${SEPOLIA_RPC_URL}',
    STAVANGER_RPC: '${STAVANGER_RPC_URL}',
    APPCHAIN_RPC: '${APPCHAIN_RPC}',
};
EOF

echo "Generated config.js with APPCHAIN_RPC=${APPCHAIN_RPC}"

# Start nginx
exec nginx -g 'daemon off;'
