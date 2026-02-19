// Auto-refresh intervals
let historyRefreshInterval = null;
let balanceRefreshInterval = null;
const HISTORY_REFRESH_MS = 2000; // 2 seconds
const BALANCE_REFRESH_MS = 5000; // 5 seconds

// CONFIG is loaded from config.js (generated from .env at container startup)

// Network metadata
const NETWORKS = {
    sepolia: {
        name: 'Sepolia',
        chainId: CONFIG.SEPOLIA_CHAIN_ID,
        icon: "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='32' height='32' viewBox='0 0 32 32'%3E%3Ccircle cx='16' cy='16' r='16' fill='%23627eea'/%3E%3Cpath d='M16 4L15.5 5.5L15.5 20.5L16 21L24 16.5z' fill='white'/%3E%3Cpath d='M16 4L8 16.5L16 21z' fill='%23c1cde9'/%3E%3C/svg%3E",
        rpcUrl: CONFIG.SEPOLIA_RPC,
        bridgeAddress: CONFIG.BRIDGE_SEPOLIA,
        tokenAddress: CONFIG.POL_TOKEN_SEPOLIA,
        tokenType: 'ERC20'
    },
    stavanger: {
        name: 'Stavanger',
        chainId: CONFIG.STAVANGER_CHAIN_ID,
        icon: "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='32' height='32' viewBox='0 0 32 32'%3E%3Ccircle cx='16' cy='16' r='16' fill='%238247e5'/%3E%3Cpath d='M10 8L16 12L22 8L16 4z' fill='white'/%3E%3Cpath d='M10 16L16 20L22 16L16 12z' fill='%23b794f6'/%3E%3Cpath d='M10 24L16 28L22 24L16 20z' fill='white'/%3E%3C/svg%3E",
        rpcUrl: CONFIG.STAVANGER_RPC,
        bridgeAddress: CONFIG.BRIDGE_STAVANGER,
        tokenAddress: '0x0000000000000000000000000000000000000000', // Native
        tokenType: 'Native'
    }
};

// Global state
let provider = null;
let signer = null;
let account = null;
let currentFromNetwork = null;
let currentToNetwork = null;
let modalTarget = null;
let fromBalance = null;
let toBalance = null;
let bridgeLiquidity = null;
let walletEventsBound = false; // Prevent duplicate event listeners

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    // Check if ethers is loaded
    if (typeof ethers === 'undefined') {
        console.error('Ethers.js not loaded! Please check your internet connection.');
        const statusDiv = document.getElementById('statusMessage');
        if (statusDiv) {
            statusDiv.className = 'status-message error';
            statusDiv.innerHTML = 'Error: Failed to load required libraries. Please refresh the page.';
            statusDiv.style.display = 'block';
        }
        return;
    }

    // Set default route: Sepolia → Stavanger
    setDefaultNetworks();

    setupEventListeners();
    checkWalletConnection();
    loadHistory();
});

function setDefaultNetworks() {
    currentFromNetwork = 'sepolia';
    currentToNetwork = 'stavanger';

    const fromNetwork = NETWORKS[currentFromNetwork];
    const toNetwork = NETWORKS[currentToNetwork];

    document.getElementById('fromIcon').src = fromNetwork.icon;
    document.getElementById('fromName').textContent = fromNetwork.name;
    document.getElementById('toIcon').src = toNetwork.icon;
    document.getElementById('toName').textContent = toNetwork.name;
}

function setupEventListeners() {
    // Tab switching
    document.querySelectorAll('.nav-link').forEach(link => {
        link.addEventListener('click', (e) => {
            const tab = e.target.dataset.tab;
            switchTab(tab);
        });
    });

    // Amount input validation
    document.getElementById('amountInput').addEventListener('input', () => {
        updateBridgeSummary();
        validateBridgeButton();
    });

    // Recipient input
    document.getElementById('recipientInput').addEventListener('input', (e) => {
        // Clear error state when typing
        e.target.classList.remove('error');
        e.target.placeholder = 'Enter address';
        validateBridgeButton();
    });
}

function switchTab(tab) {
    // Update nav links
    document.querySelectorAll('.nav-link').forEach(link => {
        link.classList.remove('active');
        if (link.dataset.tab === tab) {
            link.classList.add('active');
        }
    });

    // Update tab content
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
    });
    document.getElementById(`tab-${tab}`).classList.add('active');
}

// Wallet Connection
function toggleWallet() {
    if (account) {
        disconnectWallet();
    } else {
        connectWallet();
    }
}

async function connectWallet() {
    try {
        if (!window.ethereum) {
            showStatus('Please install MetaMask!', 'error');
            return;
        }

        // Clear disconnected flag when user manually connects
        localStorage.removeItem('walletDisconnected');

        // Request accounts
        const accounts = await window.ethereum.request({
            method: 'eth_requestAccounts'
        });

        provider = new ethers.providers.Web3Provider(window.ethereum);
        signer = provider.getSigner();
        account = accounts[0];

        // Update UI
        updateWalletUI();

        // Bind wallet events only once to prevent accumulation
        if (!walletEventsBound) {
            window.ethereum.on('accountsChanged', handleAccountsChanged);
            window.ethereum.on('chainChanged', handleChainChanged);
            walletEventsBound = true;
        }

        showStatus('Wallet connected successfully!', 'success');
        updateBalances();
        updateFaucetBalance();

        // Start balance auto-refresh
        if (!balanceRefreshInterval) {
            balanceRefreshInterval = setInterval(updateBalances, BALANCE_REFRESH_MS);
        }

    } catch (error) {
        console.error('Wallet connection error:', error);
        showStatus('Failed to connect wallet: ' + error.message, 'error');
    }
}

function handleAccountsChanged(accounts) {
    if (accounts.length === 0) {
        disconnectWallet();
    } else {
        account = accounts[0];
        updateWalletUI();
        updateBalances();
    }
}

function handleChainChanged() {
    provider = new ethers.providers.Web3Provider(window.ethereum);
    signer = provider.getSigner();
    updateBalances();
}

async function checkWalletConnection() {
    // Don't auto-connect if user manually disconnected
    if (localStorage.getItem('walletDisconnected') === 'true') {
        return;
    }
    if (window.ethereum) {
        const accounts = await window.ethereum.request({ method: 'eth_accounts' });
        if (accounts.length > 0) {
            connectWallet();
        }
    }
}

function updateWalletUI() {
    const walletBtn = document.getElementById('walletBtn');
    const changeBtn = document.getElementById('btnChangeRecipient');
    walletBtn.textContent = `${account.substring(0, 6)}...${account.substring(38)}`;
    walletBtn.classList.add('connected');
    changeBtn.style.display = 'inline';
    updateDestinationAddress();
    validateBridgeButton();
}

function updateDestinationAddress() {
    const destinationValue = document.getElementById('destinationValue');
    const recipientInput = document.getElementById('recipientInput');
    const customRecipient = recipientInput.value.trim();

    if (customRecipient && ethers.utils.isAddress(customRecipient)) {
        destinationValue.textContent = `${customRecipient.substring(0, 6)}...${customRecipient.substring(38)}`;
    } else if (account) {
        destinationValue.textContent = `${account.substring(0, 6)}...${account.substring(38)} (you)`;
    } else {
        destinationValue.textContent = 'Connect wallet';
    }
}

function toggleRecipientEdit() {
    const destinationValue = document.getElementById('destinationValue');
    const recipientInput = document.getElementById('recipientInput');
    const changeBtn = document.getElementById('btnChangeRecipient');
    const isEditing = recipientInput.style.display !== 'none';

    if (isEditing) {
        const inputValue = recipientInput.value.trim();

        // Validate if user entered something
        if (inputValue && !ethers.utils.isAddress(inputValue)) {
            // Invalid address - show error and keep editing
            recipientInput.classList.add('error');
            recipientInput.placeholder = 'Invalid address';
            return;
        }

        // Valid or empty - close editing
        recipientInput.classList.remove('error');
        recipientInput.placeholder = 'Enter address';
        recipientInput.style.display = 'none';
        destinationValue.style.display = 'inline';
        changeBtn.textContent = 'Change';
        updateDestinationAddress();
    } else {
        // Start editing - hide address and show input
        destinationValue.style.display = 'none';
        recipientInput.style.display = 'inline-block';
        recipientInput.focus();
        changeBtn.textContent = 'Done';
    }
}

function disconnectWallet() {
    provider = null;
    signer = null;
    account = null;
    localStorage.setItem('walletDisconnected', 'true');
    const walletBtn = document.getElementById('walletBtn');
    const changeBtn = document.getElementById('btnChangeRecipient');
    walletBtn.textContent = 'Connect Wallet';
    walletBtn.classList.remove('connected');
    changeBtn.style.display = 'none';
    // Reset recipient input state
    const recipientInput = document.getElementById('recipientInput');
    const destinationValue = document.getElementById('destinationValue');
    recipientInput.style.display = 'none';
    recipientInput.value = '';
    destinationValue.style.display = 'inline';
    updateDestinationAddress();
    validateBridgeButton();
}

// Network Selection
function showNetworkModal(target) {
    modalTarget = target;
    document.getElementById('networkModal').classList.add('active');
}

function closeNetworkModal() {
    document.getElementById('networkModal').classList.remove('active');
}

function selectNetwork(networkKey, isInitial = false) {
    const network = NETWORKS[networkKey];

    // Automatically select the opposite network
    const oppositeNetwork = networkKey === 'sepolia' ? 'stavanger' : 'sepolia';
    const oppositeNetworkData = NETWORKS[oppositeNetwork];

    if (modalTarget === 'from') {
        currentFromNetwork = networkKey;
        document.getElementById('fromIcon').src = network.icon;
        document.getElementById('fromName').textContent = network.name;

        // Automatically set the opposite network as "To"
        currentToNetwork = oppositeNetwork;
        document.getElementById('toIcon').src = oppositeNetworkData.icon;
        document.getElementById('toName').textContent = oppositeNetworkData.name;
    } else if (modalTarget === 'to') {
        currentToNetwork = networkKey;
        document.getElementById('toIcon').src = network.icon;
        document.getElementById('toName').textContent = network.name;

        // Automatically set the opposite network as "From"
        currentFromNetwork = oppositeNetwork;
        document.getElementById('fromIcon').src = oppositeNetworkData.icon;
        document.getElementById('fromName').textContent = oppositeNetworkData.name;
    } else if (isInitial) {
        // Initial setup
        if (!currentFromNetwork) {
            currentFromNetwork = networkKey;
            document.getElementById('fromIcon').src = network.icon;
            document.getElementById('fromName').textContent = network.name;

            // Automatically set the opposite network as "To"
            currentToNetwork = oppositeNetwork;
            document.getElementById('toIcon').src = oppositeNetworkData.icon;
            document.getElementById('toName').textContent = oppositeNetworkData.name;
        }
    }

    if (!isInitial) {
        closeNetworkModal();
        updateBalances();
        validateBridgeButton();
    }
}

async function swapDirection() {
    const temp = currentFromNetwork;
    currentFromNetwork = currentToNetwork;
    currentToNetwork = temp;

    const fromNetwork = NETWORKS[currentFromNetwork];
    const toNetwork = NETWORKS[currentToNetwork];

    document.getElementById('fromIcon').src = fromNetwork.icon;
    document.getElementById('fromName').textContent = fromNetwork.name;
    document.getElementById('toIcon').src = toNetwork.icon;
    document.getElementById('toName').textContent = toNetwork.name;

    // Auto-switch network in wallet
    if (account) {
        try {
            await switchNetwork(fromNetwork.chainId);
        } catch (error) {
            // User cancelled or network switch failed - continue anyway
        }
    }

    updateBalances();
}

// Provider cache with periodic refresh to avoid stale data
const providerCache = {};
const PROVIDER_REFRESH_MS = 30000; // Refresh providers every 30 seconds
let lastProviderRefresh = 0;

function getProvider(rpcUrl) {
    const now = Date.now();
    // Clear cache periodically to avoid stale connections
    if (now - lastProviderRefresh > PROVIDER_REFRESH_MS) {
        Object.keys(providerCache).forEach(key => delete providerCache[key]);
        lastProviderRefresh = now;
    }
    if (!providerCache[rpcUrl]) {
        providerCache[rpcUrl] = new ethers.providers.JsonRpcProvider(rpcUrl);
    }
    return providerCache[rpcUrl];
}

// Balance Management - optimized with parallel fetches
async function updateBalances() {
    if (!account || !currentFromNetwork || !currentToNetwork) return;

    try {
        const fromNetwork = NETWORKS[currentFromNetwork];
        const toNetwork = NETWORKS[currentToNetwork];

        const fromProvider = getProvider(fromNetwork.rpcUrl);
        const toProvider = getProvider(toNetwork.rpcUrl);

        // Prepare all fetch promises - use 'latest' block tag to avoid caching
        const fetchFromBalance = fromNetwork.tokenType === 'Native'
            ? fromProvider.getBalance(account, 'latest')
            : new ethers.Contract(fromNetwork.tokenAddress, ['function balanceOf(address) view returns (uint256)'], fromProvider).balanceOf(account, {blockTag: 'latest'});

        const fetchToBalance = toNetwork.tokenType === 'Native'
            ? toProvider.getBalance(account, 'latest')
            : new ethers.Contract(toNetwork.tokenAddress, ['function balanceOf(address) view returns (uint256)'], toProvider).balanceOf(account, {blockTag: 'latest'});

        const fetchLiquidity = toNetwork.tokenType === 'Native'
            ? toProvider.getBalance(toNetwork.bridgeAddress, 'latest')
            : new ethers.Contract(toNetwork.tokenAddress, ['function balanceOf(address) view returns (uint256)'], toProvider).balanceOf(toNetwork.bridgeAddress, {blockTag: 'latest'});

        // Run all fetches in parallel
        const [fromBal, toBal, liquidity] = await Promise.all([fetchFromBalance, fetchToBalance, fetchLiquidity]);

        fromBalance = fromBal;
        toBalance = toBal;
        bridgeLiquidity = liquidity;

        // Update UI
        const fromBalFormatted = parseFloat(ethers.utils.formatEther(fromBalance)).toFixed(4);
        const toBalFormatted = parseFloat(ethers.utils.formatEther(toBalance)).toFixed(4);
        const liqFormatted = parseFloat(ethers.utils.formatEther(liquidity)).toFixed(2);

        document.getElementById('fromBalance').textContent = `Balance: ${fromBalFormatted} POL`;
        document.getElementById('toBalance').textContent = `Balance: ${toBalFormatted} POL`;
        document.getElementById('bridgeLiquidity').textContent = `Liquidity: ${liqFormatted} POL`;

        console.log(`[Balance] From: ${fromBalFormatted}, To: ${toBalFormatted}, Liq: ${liqFormatted}`);

        validateBridgeButton();

    } catch (error) {
        console.error('Balance update error:', error);
        document.getElementById('fromBalance').textContent = 'Balance: Error';
        document.getElementById('toBalance').textContent = 'Balance: Error';
        document.getElementById('bridgeLiquidity').textContent = 'Liquidity: Error';
    }
}

function setMaxAmount() {
    if (!fromBalance || !currentFromNetwork) {
        showStatus('Please wait for balance to load', 'info');
        return;
    }

    const fromNetwork = NETWORKS[currentFromNetwork];
    let maxAmount;

    if (fromNetwork.tokenType === 'Native') {
        // Reserve some for gas (0.01 POL)
        const gasReserve = ethers.utils.parseEther('0.01');
        maxAmount = fromBalance.sub(gasReserve);

        if (maxAmount.lt(0)) {
            maxAmount = ethers.BigNumber.from(0);
        }
    } else {
        // For ERC20, can use full balance
        maxAmount = fromBalance;
    }

    const formattedAmount = ethers.utils.formatEther(maxAmount);
    // Clean up floating point precision issues
    const cleanAmount = parseFloat(formattedAmount).toString();
    document.getElementById('amountInput').value = cleanAmount;

    updateBridgeSummary();
    validateBridgeButton();
}

// Bridge Summary
function updateBridgeSummary() {
    const amount = document.getElementById('amountInput').value;
    if (amount && parseFloat(amount) > 0) {
        document.getElementById('bridgeSummary').style.display = 'block';
        document.getElementById('receiveAmount').textContent = `${amount} POL`;
    } else {
        document.getElementById('bridgeSummary').style.display = 'none';
    }
}

// Bridge Validation
function validateBridgeButton() {
    const amount = document.getElementById('amountInput').value;
    const recipient = document.getElementById('recipientInput').value || account;
    const bridgeBtn = document.getElementById('bridgeBtn');

    if (!account) {
        bridgeBtn.disabled = true;
        bridgeBtn.textContent = 'Connect Wallet to Bridge';
    } else if (!currentFromNetwork || !currentToNetwork) {
        bridgeBtn.disabled = true;
        bridgeBtn.textContent = 'Select Networks';
    } else if (!amount || parseFloat(amount) <= 0) {
        bridgeBtn.disabled = true;
        bridgeBtn.textContent = 'Enter Amount';
    } else if (!recipient || !ethers.utils.isAddress(recipient)) {
        bridgeBtn.disabled = true;
        bridgeBtn.textContent = 'Invalid Recipient';
    } else if (bridgeLiquidity && ethers.utils.parseEther(amount).gt(bridgeLiquidity)) {
        bridgeBtn.disabled = true;
        bridgeBtn.textContent = '⚠️ Insufficient Liquidity';
    } else {
        bridgeBtn.disabled = false;
        bridgeBtn.textContent = '⚡ Bridge with Permit';
    }
}

// EIP-2612 Permit Signature
async function getPermitSignature(tokenAddress, spender, value, deadline) {
    try {
        const token = new ethers.Contract(
            tokenAddress,
            [
                'function name() view returns (string)',
                'function nonces(address) view returns (uint256)',
            ],
            signer
        );

        const [name, nonce] = await Promise.all([
            token.name(),
            token.nonces(account)
        ]);

        const chainId = await signer.getChainId();

        // EIP-712 domain
        const domain = {
            name: name,
            version: '1',
            chainId: chainId,
            verifyingContract: tokenAddress
        };

        // EIP-712 types
        const types = {
            Permit: [
                { name: 'owner', type: 'address' },
                { name: 'spender', type: 'address' },
                { name: 'value', type: 'uint256' },
                { name: 'nonce', type: 'uint256' },
                { name: 'deadline', type: 'uint256' }
            ]
        };

        // Message
        const message = {
            owner: account,
            spender: spender,
            value: value.toString(),
            nonce: nonce.toString(),
            deadline: deadline
        };

        // Sign
        const signature = await signer._signTypedData(domain, types, message);
        const sig = ethers.utils.splitSignature(signature);

        return { v: sig.v, r: sig.r, s: sig.s };

    } catch (error) {
        console.error('Permit signature error:', error);
        throw error;
    }
}

// Bridge with Permit
async function initiateBridge() {
    try {
        const amount = document.getElementById('amountInput').value;
        const recipient = document.getElementById('recipientInput').value || account;
        const fromNetwork = NETWORKS[currentFromNetwork];
        const toNetwork = NETWORKS[currentToNetwork];

        if (!amount || !recipient) {
            showStatus('Please fill in all fields', 'error');
            return;
        }

        // Disable button
        document.getElementById('bridgeBtn').disabled = true;
        showStatus('Preparing bridge transaction...', 'info');

        const amountWei = ethers.utils.parseEther(amount);

        // Check if we're on the correct network
        const chainId = await signer.getChainId();
        if (chainId !== fromNetwork.chainId) {
            await switchNetwork(fromNetwork.chainId);
        }

        let permitData = '0x';

        // Get permit signature for ERC20 tokens
        if (fromNetwork.tokenType === 'ERC20') {
            try {
                showStatus('📝 Please sign the permit message...', 'info');
                const deadline = Math.floor(Date.now() / 1000) + 3600; // 1 hour

                const { v, r, s } = await getPermitSignature(
                    fromNetwork.tokenAddress,
                    fromNetwork.bridgeAddress,
                    amountWei,
                    deadline
                );

                // Encode permit data
                permitData = ethers.utils.defaultAbiCoder.encode(
                    ['address', 'address', 'uint256', 'uint256', 'uint8', 'bytes32', 'bytes32'],
                    [account, fromNetwork.bridgeAddress, amountWei, deadline, v, r, s]
                );
            } catch (error) {
                // Permit failed, falling back to approval
                showStatus('Getting token approval...', 'info');

                const token = new ethers.Contract(
                    fromNetwork.tokenAddress,
                    ['function approve(address, uint256)'],
                    signer
                );

                const approveTx = await token.approve(fromNetwork.bridgeAddress, amountWei);
                await approveTx.wait();
            }
        }

        // Bridge transaction
        showStatus('🚀 Submitting bridge transaction...', 'info');

        const bridge = new ethers.Contract(
            fromNetwork.bridgeAddress,
            [
                'function bridgeAsset(address, uint256, uint256, address, bytes) payable returns (bytes32)'
            ],
            signer
        );

        const txOptions = fromNetwork.tokenType === 'Native' ?
            { value: amountWei } : {};

        const tx = await bridge.bridgeAsset(
            fromNetwork.tokenAddress,
            amountWei,
            toNetwork.chainId,
            recipient,
            permitData,
            txOptions
        );

        showStatus('⏳ Waiting for confirmation...', 'info');
        const receipt = await tx.wait();

        // Extract bridgeId from BridgeInitiated event
        let bridgeId = null;
        if (receipt.logs && receipt.logs.length > 0) {
            // BridgeInitiated event signature
            const bridgeInitiatedTopic = '0xa43a2e0bb4454dc2f20f4a34be7549f0e1b00e4f5e88805c729900e40471a0cb';
            const bridgeLog = receipt.logs.find(log => log.topics[0] === bridgeInitiatedTopic);
            if (bridgeLog && bridgeLog.topics.length >= 2) {
                bridgeId = bridgeLog.topics[1]; // bridgeId is the first indexed parameter
            }
        }

        // Save to history
        saveTransaction({
            hash: receipt.transactionHash,
            bridgeId: bridgeId,
            from: fromNetwork.name,
            to: toNetwork.name,
            amount: amount,
            timestamp: Date.now(),
            status: 'Pending' // Initial status, will be updated by RPC query
        });

        showStatus(
            `✅ Bridge successful!<br>Transaction: <a href="https://${currentFromNetwork === 'sepolia' ? 'sepolia.etherscan.io' : 'explorer.stavanger.gateway.fm'}/tx/${receipt.transactionHash}" target="_blank">${receipt.transactionHash.substring(0, 10)}...</a>`,
            'success'
        );

        // Clear form
        document.getElementById('amountInput').value = '';
        updateBridgeSummary();
        updateBalances();
        loadHistory();

    } catch (error) {
        console.error('Bridge error:', error);
        showStatus('❌ Bridge failed: ' + error.message, 'error');
    } finally {
        validateBridgeButton();
    }
}

async function switchNetwork(chainId) {
    try {
        await window.ethereum.request({
            method: 'wallet_switchEthereumChain',
            params: [{ chainId: '0x' + chainId.toString(16) }],
        });
    } catch (error) {
        if (error.code === 4902) {
            // Chain not added, add it
            if (chainId === CONFIG.STAVANGER_CHAIN_ID) {
                await window.ethereum.request({
                    method: 'wallet_addEthereumChain',
                    params: [{
                        chainId: '0x' + chainId.toString(16),
                        chainName: 'Stavanger',
                        nativeCurrency: {
                            name: 'POL',
                            symbol: 'POL',
                            decimals: 18
                        },
                        rpcUrls: [CONFIG.STAVANGER_RPC],
                        blockExplorerUrls: ['https://explorer.stavanger.gateway.fm']
                    }]
                });
            }
        } else {
            throw error;
        }
    }
}

// Transaction History
function saveTransaction(tx) {
    const history = JSON.parse(localStorage.getItem('bridgeHistory') || '[]');
    history.unshift(tx);
    localStorage.setItem('bridgeHistory', JSON.stringify(history.slice(0, 50))); // Keep last 50
}

// Query bridge status from appchain RPC
async function queryBridgeStatus(bridgeId) {
    if (!bridgeId) return null;

    try {
        const response = await fetch(CONFIG.APPCHAIN_RPC, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                jsonrpc: '2.0',
                method: 'getBridgeStatus',
                params: [{ bridgeId }],
                id: 1
            })
        });

        const data = await response.json();
        if (data.result) {
            return {
                status: data.result.status,
                claimTxHash: data.result.claimTxHash || null
            };
        }
    } catch (error) {
        // Silently fail - appchain might not be running
    }

    return null;
}

async function loadHistory() {
    const history = JSON.parse(localStorage.getItem('bridgeHistory') || '[]');
    const historyList = document.getElementById('historyList');

    if (history.length === 0) {
        historyList.innerHTML = `
            <div class="empty-state">
                <svg width="64" height="64" viewBox="0 0 64 64" fill="none">
                    <circle cx="32" cy="32" r="30" stroke="#e5e7eb" stroke-width="4"/>
                    <path d="M32 16V32L42 42" stroke="#9ca3af" stroke-width="4" stroke-linecap="round"/>
                </svg>
                <p>No transactions yet</p>
                <span>Your bridge transactions will appear here</span>
            </div>
        `;
        return;
    }

    // Query real status for each transaction
    const updatedHistory = await Promise.all(history.map(async (tx) => {
        if (tx.bridgeId && tx.status !== 'Completed' && tx.status !== 'Failed') {
            const rpcResult = await queryBridgeStatus(tx.bridgeId);
            if (rpcResult) {
                tx.status = rpcResult.status;
                if (rpcResult.claimTxHash) {
                    tx.claimTxHash = rpcResult.claimTxHash;
                }
            }
        }
        return tx;
    }));

    // Update localStorage with real statuses
    localStorage.setItem('bridgeHistory', JSON.stringify(updatedHistory));

    // Auto-refresh: start interval if there are pending transactions, stop if all completed
    const hasPending = updatedHistory.some(tx => tx.status !== 'Completed' && tx.status !== 'Failed');
    if (hasPending && !historyRefreshInterval) {
        historyRefreshInterval = setInterval(loadHistory, HISTORY_REFRESH_MS);
    } else if (!hasPending && historyRefreshInterval) {
        clearInterval(historyRefreshInterval);
        historyRefreshInterval = null;
    }

    // Group transactions by status
    const pending = updatedHistory.filter(tx => tx.status === 'Pending');
    const confirmed = updatedHistory.filter(tx => tx.status === 'Confirmed');
    const failed = updatedHistory.filter(tx => tx.status === 'Failed');
    const completed = updatedHistory.filter(tx => tx.status === 'Completed');

    const renderTx = (tx) => {
        const sourceExplorerUrl = tx.from === 'Sepolia'
            ? `https://sepolia.etherscan.io/tx/${tx.hash}`
            : `https://explorer.stavanger.gateway.fm/tx/${tx.hash}`;

        const destExplorerBase = tx.from === 'Sepolia'
            ? 'https://explorer.stavanger.gateway.fm/tx/'
            : 'https://sepolia.etherscan.io/tx/';

        let txLinksHtml = `<a href="${sourceExplorerUrl}" target="_blank" onclick="event.stopPropagation();" style="color: #6366f1; text-decoration: none;">Source Tx</a>`;

        if (tx.claimTxHash) {
            txLinksHtml += ` | <a href="${destExplorerBase}${tx.claimTxHash}" target="_blank" onclick="event.stopPropagation();" style="color: #10b981; text-decoration: none;">Claim Tx</a>`;
        }

        return `
            <div class="history-item">
                <div class="history-info">
                    <div class="history-icon">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M7 10L12 15L17 10M17 14L12 9L7 14"/>
                        </svg>
                    </div>
                    <div class="history-details">
                        <h4>${tx.amount} POL - ${tx.from} → ${tx.to}</h4>
                        <p>${new Date(tx.timestamp).toLocaleString()}</p>
                        ${tx.bridgeId ? `<p class="bridge-id">Bridge ID: ${tx.bridgeId.substring(0, 10)}...</p>` : ''}
                        <p class="tx-links" style="font-size: 0.75rem; margin-top: 0.25rem;">
                            ${txLinksHtml}
                        </p>
                    </div>
                </div>
                <div class="history-status">
                    <span class="status-badge ${tx.status.toLowerCase()}">${tx.status}</span>
                </div>
            </div>
        `;
    };

    const renderSection = (title, icon, txs, colorClass) => {
        if (txs.length === 0) return '';
        return `
            <div class="history-section">
                <div class="history-section-header ${colorClass}">
                    ${icon}
                    <span>${title} (${txs.length})</span>
                </div>
                ${txs.map(renderTx).join('')}
            </div>
        `;
    };

    historyList.innerHTML =
        renderSection('Pending', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>', pending, 'pending') +
        renderSection('Bridged', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg>', confirmed, 'confirmed') +
        renderSection('Failed', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M15 9l-6 6M9 9l6 6"/></svg>', failed, 'failed') +
        renderSection('Claimed', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6L9 17l-5-5"/></svg>', completed, 'completed');
}

// Clear History
function clearHistory() {
    localStorage.removeItem('bridgeHistory');
    if (historyRefreshInterval) {
        clearInterval(historyRefreshInterval);
        historyRefreshInterval = null;
    }
    loadHistory();
}

// Faucet - Mint test POL tokens on Sepolia
async function mintTestPOL() {
    try {
        if (!account) {
            showFaucetStatus('Please connect your wallet first', 'error');
            return;
        }

        const faucetBtn = document.getElementById('faucetBtn');
        faucetBtn.disabled = true;
        faucetBtn.textContent = 'Minting...';

        // Check if on Sepolia and switch if needed
        const chainId = await signer.getChainId();
        if (chainId !== CONFIG.SEPOLIA_CHAIN_ID) {
            showFaucetStatus('Switching to Sepolia...', 'info');
            await switchNetwork(CONFIG.SEPOLIA_CHAIN_ID);
            // Refresh provider and signer after network switch
            provider = new ethers.providers.Web3Provider(window.ethereum);
            signer = provider.getSigner();
        }

        showFaucetStatus('Minting 10 test POL tokens...', 'info');

        const polToken = new ethers.Contract(
            CONFIG.POL_TOKEN_SEPOLIA,
            ['function mint(address to, uint256 amount)'],
            signer
        );

        const mintAmount = ethers.utils.parseEther('10'); // 10 POL
        const tx = await polToken.mint(account, mintAmount);

        showFaucetStatus('Waiting for confirmation...', 'info');
        await tx.wait();

        showFaucetStatus('Successfully minted 10 test POL!', 'success');
        updateBalances();
        updateFaucetBalance();

    } catch (error) {
        console.error('Faucet error:', error);
        showFaucetStatus('Faucet failed: ' + error.message, 'error');
    } finally {
        const faucetBtn = document.getElementById('faucetBtn');
        faucetBtn.disabled = false;
        faucetBtn.textContent = 'Get 10 Test POL';
    }
}

// Show status in faucet tab
function showFaucetStatus(message, type) {
    const statusDiv = document.getElementById('faucetStatus');
    statusDiv.className = `faucet-status ${type}`;
    statusDiv.textContent = message;
    statusDiv.style.display = 'block';

    if (type === 'success') {
        setTimeout(() => {
            statusDiv.style.display = 'none';
        }, 5000);
    }
}

// Update faucet balance display
async function updateFaucetBalance() {
    const faucetBalanceEl = document.getElementById('faucetBalance');
    if (!account) {
        faucetBalanceEl.textContent = 'Connect wallet';
        return;
    }

    try {
        const sepoliaProvider = getProvider(CONFIG.SEPOLIA_RPC);
        const polToken = new ethers.Contract(
            CONFIG.POL_TOKEN_SEPOLIA,
            ['function balanceOf(address) view returns (uint256)'],
            sepoliaProvider
        );
        const balance = await polToken.balanceOf(account);
        const formatted = parseFloat(ethers.utils.formatEther(balance)).toFixed(4);
        faucetBalanceEl.textContent = `${formatted} POL`;
    } catch (error) {
        faucetBalanceEl.textContent = 'Error loading';
    }
}

// Status Messages
function showStatus(message, type) {
    const statusDiv = document.getElementById('statusMessage');
    statusDiv.className = `status-message ${type}`;
    statusDiv.innerHTML = message;
    statusDiv.style.display = 'block';

    if (type === 'success') {
        setTimeout(() => {
            statusDiv.style.display = 'none';
        }, 10000);
    }
}
