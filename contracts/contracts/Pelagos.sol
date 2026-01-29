// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title Pelagos Contract
 * @notice Routes external transactions from appchains to registered contracts
 * @dev Handles cross-chain transactions through validator consensus
 */
contract Pelagos {

    // Event emitted when an external transaction is processed
    event ExternalTransactionProcessed(
        uint256 indexed appChainId,
        uint256 indexed sequenceNumber,
        bytes32 indexed nonceHash,
        bytes payload
    );

    // Event emitted when ownership is transferred
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    // Event emitted when an appchain is registered
    event AppchainRegistered(uint256 indexed appChainId, address indexed appchainContract);

    // Event emitted when an appchain is unregistered
    event AppchainUnregistered(uint256 indexed appChainId);

    // Event emitted when transaction received but no appchain registered
    event AppchainNotRegistered(uint256 indexed appChainId, bytes payload);

    // Contract owner (should be set to the consensus system)
    address public owner;

    // Mapping to track processed transactions to prevent replay attacks
    mapping(bytes32 => bool) public processedTransactions;

    // Counter for total processed transactions
    uint256 public totalProcessedTransactions;

    // Mapping from appchain ID to appchain contract address
    mapping(uint256 => address) public appchainContracts;

    error CallerNotOwner();
    error InvalidAppChainId();
    error InvalidNonceHash();
    error EmptyPayload();
    error TransactionAlreadyProcessed();
    error InvalidOwner();
    error InvalidAppchainContract();
    error AppchainCallFailed();

    modifier onlyOwner() {
        if (msg.sender != owner) revert CallerNotOwner();
        _;
    }

    constructor() {
        owner = msg.sender;
        emit OwnershipTransferred(address(0), msg.sender);
    }

    /**
     * @dev Transfer ownership of the contract
     * @param newOwner The address of the new owner
     */
    function transferOwnership(address newOwner) external onlyOwner {
        if (newOwner == address(0)) revert InvalidOwner();
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    /**
     * @dev Process an external transaction from an appchain
     * @param appChainId The chain ID of the source appchain
     * @param nonceHash The deterministic hash for replay protection
     * @param payload The encoded transaction payload
     */
    function processExternalTransaction(
        uint256 appChainId,
        bytes32 nonceHash,
        bytes calldata payload
    ) external onlyOwner {
        if (appChainId == 0) revert InvalidAppChainId();
        if (nonceHash == bytes32(0)) revert InvalidNonceHash();
        if (payload.length == 0) revert EmptyPayload();

        // Prevent replay attacks using deterministic nonce hash
        if (processedTransactions[nonceHash]) revert TransactionAlreadyProcessed();
        processedTransactions[nonceHash] = true;

        // Increment counter
        totalProcessedTransactions++;

        // Emit event for indexing and monitoring
        emit ExternalTransactionProcessed(appChainId, totalProcessedTransactions, nonceHash, payload);

        // Route to appropriate appchain contract based on appChainId
        _processPayload(appChainId, payload);
    }

    /**
     * @dev Internal function to process the payload by routing to appropriate appchain contract
     * @param appChainId The source chain ID
     * @param payload The transaction payload
     */
    function _processPayload(uint256 appChainId, bytes calldata payload) internal {
        address appchainContract = appchainContracts[appChainId];

        if (appchainContract != address(0)) {
            // Route to specific appchain contract (Bridge implements executeTransaction)
            (bool success,) = appchainContract.call(
                abi.encodeWithSignature("executeTransaction(bytes)", payload)
            );

            if (!success) revert AppchainCallFailed();
        } else {
            // No specific appchain contract registered, emit event for monitoring
            emit AppchainNotRegistered(appChainId, payload);
        }
    }

    /**
     * @dev Register an appchain contract for a specific appchain ID
     * @param appChainId The appchain ID to register
     * @param appchainContract The appchain contract address (e.g., Bridge contract)
     */
    function registerAppchainContract(uint256 appChainId, address appchainContract) external onlyOwner {
        if (appChainId == 0) revert InvalidAppChainId();
        if (appchainContract == address(0)) revert InvalidAppchainContract();

        appchainContracts[appChainId] = appchainContract;
        emit AppchainRegistered(appChainId, appchainContract);
    }

    /**
     * @dev Unregister an appchain contract for a specific appchain ID
     * @param appChainId The appchain ID to unregister
     */
    function unregisterAppchainContract(uint256 appChainId) external onlyOwner {
        delete appchainContracts[appChainId];
        emit AppchainUnregistered(appChainId);
    }
}
