// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/IERC20Permit.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/utils/Pausable.sol";

/**
 * @title Bridge
 * @notice Generic cross-chain bridge supporting any ERC20 token or native gas token
 * @dev Integrates with Pelagos SDK - implements executeTransaction(bytes) for cross-chain calls
 *
 * Architecture:
 * - Supports any ERC20 token on any chain
 * - Native gas token represented by address(0)
 * - Pelagos.sol calls executeTransaction() to complete bridge on destination
 *
 * Flow:
 * Source -> Destination:
 *   1. User calls bridgeAsset(token, amount, destChainId, recipient)
 *   2. For ERC20: tokens locked in bridge, for native: msg.value locked
 *   3. BridgeInitiated event emitted
 *   4. pelacli monitors event, processes in appchain
 *   5. Appchain generates ExternalTransaction with binary payload
 *   6. pelacli submits to Pelagos.sol on destination chain
 *   7. Pelagos.sol calls Bridge.executeTransaction(payload)
 *   8. Bridge unlocks tokens to recipient
 *
 * Token Identification:
 * - address(0) = native gas token (ETH, POL, etc.)
 * - Any other address = ERC20 token contract
 */
contract Bridge is Ownable, ReentrancyGuard, Pausable {
    using SafeERC20 for IERC20;

    // ============ State Variables ============

    /// @notice Pelagos contract address (authorized to call executeTransaction)
    address public pelagosContract;

    /// @notice Current chain ID
    uint256 public immutable chainId;

    /// @notice Nonce for generating unique bridge IDs
    uint256 public bridgeNonce;

    /// @notice Mapping of bridge ID to whether it has been claimed
    mapping(bytes32 => bool) public claimedBridges;

    /// @notice Mapping of bridge ID to bridge details
    mapping(bytes32 => BridgeData) public bridges;

    // ============ Structs ============

    struct BridgeData {
        uint256 sourceChainId;
        uint256 destChainId;
        address token;
        uint256 amount;
        address sender;
        address recipient;
        uint256 timestamp;
        bool claimed;
    }

    // ============ Events ============

    /**
     * @notice Emitted when a bridge is initiated
     * @param bridgeId Unique identifier for this bridge transaction
     * @param sourceChainId Chain ID where bridge was initiated
     * @param destChainId Target chain ID
     * @param token Token address (address(0) for native)
     * @param amount Amount of tokens
     * @param sender Original sender on source chain
     * @param recipient Recipient address on destination chain
     */
    event BridgeInitiated(
        bytes32 indexed bridgeId,
        uint256 indexed sourceChainId,
        uint256 indexed destChainId,
        address token,
        uint256 amount,
        address sender,
        address recipient
    );

    /**
     * @notice Emitted when tokens are claimed on destination chain
     * @param bridgeId Unique identifier for this bridge transaction
     * @param recipient Recipient who received tokens
     * @param token Token address (address(0) for native)
     * @param amount Amount claimed
     */
    event AssetClaimed(
        bytes32 indexed bridgeId,
        address indexed recipient,
        address indexed token,
        uint256 amount
    );

    /**
     * @notice Emitted when Pelagos contract is updated
     */
    event PelagosContractUpdated(address indexed oldPelagos, address indexed newPelagos);

    // ============ Errors ============

    error InvalidDestinationChain();
    error InvalidRecipient();
    error InvalidToken();
    error InvalidAmount();
    error InvalidBridgeId();
    error BridgeAlreadyClaimed();
    error InsufficientBalance();
    error Unauthorized();
    error PayloadDecodingFailed();
    error NativeTransferFailed();
    error PermitFailed();

    // ============ Modifiers ============

    /**
     * @notice Only Pelagos contract can call executeTransaction
     */
    modifier onlyPelagos() {
        if (msg.sender != pelagosContract) revert Unauthorized();
        _;
    }

    // ============ Constructor ============

    /**
     * @notice Initialize bridge contract
     */
    constructor() Ownable(msg.sender) {
        chainId = block.chainid;
    }

    // ============ External Functions ============

    /**
     * @notice Bridge tokens to destination chain
     * @param token Token address to bridge (address(0) for native gas token)
     * @param amount Amount of tokens to bridge
     * @param destChainId Destination chain ID
     * @param recipient Recipient address on destination chain
     * @param permitData Optional EIP-2612 permit data for gasless approval (empty for pre-approved or native tokens)
     * @return bridgeId Unique identifier for this bridge transaction
     *
     * @dev For native tokens: Send value with transaction (msg.value must equal amount)
     *      For ERC20 tokens: Either approve beforehand OR provide permitData for one-transaction bridging
     *
     *      permitData format (if using permit):
     *      - Encode: abi.encode(owner, spender, value, deadline, v, r, s)
     *      - This executes permit() before transferFrom, enabling one-transaction bridging UX
     */
    function bridgeAsset(
        address token,
        uint256 amount,
        uint256 destChainId,
        address recipient,
        bytes calldata permitData
    ) external payable nonReentrant whenNotPaused returns (bytes32 bridgeId) {
        // Validation
        if (destChainId == chainId) revert InvalidDestinationChain();
        if (recipient == address(0)) revert InvalidRecipient();
        if (amount == 0) revert InvalidAmount();

        if (token == address(0)) {
            // Native gas token (ETH, POL, etc.)
            if (msg.value != amount) revert InvalidAmount();
            // Native token is already received via msg.value
        } else {
            // ERC20 token
            // No msg.value should be sent for ERC20
            if (msg.value != 0) revert InvalidAmount();

            // If permit data is provided, execute permit for gasless approval
            if (permitData.length > 0) {
                try this._executePermit(token, permitData) {
                    // Permit executed successfully
                } catch {
                    // Permit failed - user should have pre-approved or permit already used
                    // Continue with transferFrom, which will revert if no allowance
                }
            }

            // Transfer tokens from user to bridge
            IERC20(token).safeTransferFrom(msg.sender, address(this), amount);
        }

        // Generate unique bridge ID
        bridgeId = keccak256(
            abi.encodePacked(
                chainId,
                destChainId,
                token,
                amount,
                msg.sender,
                recipient,
                bridgeNonce,
                block.timestamp
            )
        );

        // Increment nonce
        bridgeNonce++;

        // Store bridge data
        bridges[bridgeId] = BridgeData({
            sourceChainId: chainId,
            destChainId: destChainId,
            token: token,
            amount: amount,
            sender: msg.sender,
            recipient: recipient,
            timestamp: block.timestamp,
            claimed: false
        });

        // Emit event (monitored by pelacli)
        emit BridgeInitiated(
            bridgeId,
            chainId,
            destChainId,
            token,
            amount,
            msg.sender,
            recipient
        );
    }

    /**
     * @notice Execute cross-chain transaction from Pelagos
     * @param payload Binary payload containing bridge claim data
     * @dev Called by Pelagos.sol when processing external transaction from appchain
     *
     * Payload structure (160 bytes):
     * - Bytes 0-31:    bridgeId (bytes32)
     * - Bytes 32-63:   sourceChainId (uint256)
     * - Bytes 64-95:   token (address, LEFT-aligned: [20_bytes_address][12_bytes_zeros])
     * - Bytes 96-127:  amount (uint256)
     * - Bytes 128-159: recipient (address, LEFT-aligned: [20_bytes_address][12_bytes_zeros])
     *
     * Note: Addresses are LEFT-aligned so shr(96) extracts them correctly
     * Example: [0xABCD...][000...] -> shr(96) -> [000...][0xABCD...] -> address(0xABCD...)
     */
    function executeTransaction(bytes calldata payload) external onlyPelagos nonReentrant {
        // Decode payload using assembly for gas efficiency
        bytes32 bridgeId;
        uint256 sourceChainId;
        address token;
        uint256 amount;
        address recipient;

        // Validate payload length (160 bytes = 5 * 32 bytes)
        if (payload.length != 160) revert PayloadDecodingFailed();

        assembly {
            // bridgeId: bytes 0-31
            bridgeId := calldataload(payload.offset)

            // sourceChainId: bytes 32-63
            sourceChainId := calldataload(add(payload.offset, 32))

            // token: bytes 64-95 (address LEFT-aligned, extract with shr(96))
            token := shr(96, calldataload(add(payload.offset, 64)))

            // amount: bytes 96-127
            amount := calldataload(add(payload.offset, 96))

            // recipient: bytes 128-159 (address LEFT-aligned, extract with shr(96))
            recipient := shr(96, calldataload(add(payload.offset, 128)))
        }

        // Validate
        if (bridgeId == bytes32(0)) revert InvalidBridgeId();
        if (recipient == address(0)) revert InvalidRecipient();
        if (amount == 0) revert InvalidAmount();

        // Check if already claimed
        if (claimedBridges[bridgeId]) revert BridgeAlreadyClaimed();

        // Mark as claimed
        claimedBridges[bridgeId] = true;

        // Transfer tokens to recipient
        if (token == address(0)) {
            // Send native gas token
            if (address(this).balance < amount) revert InsufficientBalance();
            (bool success, ) = recipient.call{value: amount}("");
            if (!success) revert NativeTransferFailed();
        } else {
            // Transfer ERC20 token
            if (IERC20(token).balanceOf(address(this)) < amount) revert InsufficientBalance();
            IERC20(token).safeTransfer(recipient, amount);
        }

        // Emit event
        emit AssetClaimed(bridgeId, recipient, token, amount);
    }

    // ============ Admin Functions ============

    /**
     * @notice Set Pelagos contract address
     * @param _pelagosContract Address of Pelagos contract
     */
    function setPelagosContract(address _pelagosContract) external onlyOwner {
        if (_pelagosContract == address(0)) revert InvalidRecipient();
        address old = pelagosContract;
        pelagosContract = _pelagosContract;
        emit PelagosContractUpdated(old, _pelagosContract);
    }

    /**
     * @notice Pause bridge operations
     */
    function pause() external onlyOwner {
        _pause();
    }

    /**
     * @notice Unpause bridge operations
     */
    function unpause() external onlyOwner {
        _unpause();
    }

    /**
     * @notice Emergency withdraw of tokens (only owner)
     * @param token Token address (address(0) for native)
     * @param amount Amount to withdraw
     */
    function emergencyWithdraw(address token, uint256 amount) external onlyOwner {
        if (token == address(0)) {
            // Withdraw native
            (bool success, ) = owner().call{value: amount}("");
            if (!success) revert NativeTransferFailed();
        } else {
            // Withdraw ERC20
            IERC20(token).safeTransfer(owner(), amount);
        }
    }

    /**
     * @notice Add liquidity to bridge
     * @param token Token address (address(0) for native)
     * @param amount Amount to add
     */
    function addLiquidity(address token, uint256 amount) external payable onlyOwner {
        if (token == address(0)) {
            // Native token - must send value with transaction
            if (msg.value != amount) revert InvalidAmount();
        } else {
            // ERC20 token
            if (msg.value != 0) revert InvalidAmount();
            IERC20(token).safeTransferFrom(msg.sender, address(this), amount);
        }
    }

    // ============ Receive Functions ============

    /**
     * @notice Receive native tokens for liquidity
     */
    receive() external payable {
        // Accept native tokens for liquidity
    }

    /**
     * @notice Fallback function
     */
    fallback() external payable {
        revert("Bridge: invalid function call");
    }

    // ============ Internal Functions ============

    /**
     * @notice Execute EIP-2612 permit for gasless approval
     * @param token Token contract address
     * @param permitData Encoded permit parameters
     * @dev This is external only to enable try-catch from bridgeAsset
     */
    function _executePermit(address token, bytes calldata permitData) external {
        // Only callable by this contract (via try-catch in bridgeAsset)
        if (msg.sender != address(this)) revert Unauthorized();

        // Decode permit parameters
        (address owner, address spender, uint256 value, uint256 deadline, uint8 v, bytes32 r, bytes32 s) =
            abi.decode(permitData, (address, address, uint256, uint256, uint8, bytes32, bytes32));

        // Execute permit
        IERC20Permit(token).permit(owner, spender, value, deadline, v, r, s);
    }

    // ============ View Functions ============

    /**
     * @notice Get bridge data
     * @param bridgeId Bridge identifier
     * @return Bridge data struct
     */
    function getBridge(bytes32 bridgeId) external view returns (BridgeData memory) {
        return bridges[bridgeId];
    }

    /**
     * @notice Check if bridge has been claimed
     * @param bridgeId Bridge identifier
     * @return Whether bridge has been claimed
     */
    function isClaimed(bytes32 bridgeId) external view returns (bool) {
        return claimedBridges[bridgeId];
    }

    /**
     * @notice Get contract balance for a specific token
     * @param token Token address (address(0) for native)
     * @return Balance of specified token
     */
    function getBalance(address token) external view returns (uint256) {
        if (token == address(0)) {
            return address(this).balance;
        } else {
            return IERC20(token).balanceOf(address(this));
        }
    }
}
