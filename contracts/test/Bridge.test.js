const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("Bridge", function () {
  let bridge;
  let pelagos;
  let mockToken;
  let owner;
  let user1;
  let user2;

  const AMOY_CHAIN_ID = 80002;
  const APP_CHAIN_ID = 1604;

  beforeEach(async function () {
    [owner, user1, user2] = await ethers.getSigners();

    // Deploy mock token
    const MockERC20 = await ethers.getContractFactory("MockERC20");
    mockToken = await MockERC20.deploy("Test USDT", "USDT", 6);
    await mockToken.waitForDeployment();

    // Mint tokens to user1
    await mockToken.mint(user1.address, ethers.parseUnits("10000", 6));

    // Deploy Pelagos
    const Pelagos = await ethers.getContractFactory("Pelagos");
    pelagos = await Pelagos.deploy();
    await pelagos.waitForDeployment();

    // Deploy Bridge
    const Bridge = await ethers.getContractFactory("Bridge");
    bridge = await Bridge.deploy();
    await bridge.waitForDeployment();

    // Wire them together
    await bridge.setPelagosContract(await pelagos.getAddress());
    await pelagos.registerAppchainContract(APP_CHAIN_ID, await bridge.getAddress());
  });

  describe("Deployment", function () {
    it("Should set the correct chainId", async function () {
      expect(await bridge.chainId()).to.equal(31337); // Hardhat network
    });

    it("Should set the correct owner", async function () {
      expect(await bridge.owner()).to.equal(owner.address);
    });

    it("Should set the correct Pelagos contract", async function () {
      expect(await bridge.pelagosContract()).to.equal(await pelagos.getAddress());
    });
  });

  describe("Pelagos", function () {
    it("Should register bridge with Pelagos", async function () {
      expect(await pelagos.appchainContracts(APP_CHAIN_ID)).to.equal(await bridge.getAddress());
    });

    it("Should have owner as Pelagos owner", async function () {
      expect(await pelagos.owner()).to.equal(owner.address);
    });
  });

  describe("BridgeAsset - ERC20", function () {
    it("Should lock tokens and emit BridgeInitiated event", async function () {
      const amount = ethers.parseUnits("1000", 6);

      // Approve bridge to spend tokens
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      // Bridge tokens
      const tx = await bridge.connect(user1).bridgeAsset(
        await mockToken.getAddress(),
        amount,
        AMOY_CHAIN_ID,
        user2.address,
        "0x" // no permit data
      );

      const receipt = await tx.wait();
      const event = receipt.logs.find(
        log => log.fragment && log.fragment.name === "BridgeInitiated"
      );

      expect(event).to.not.be.undefined;
      expect(event.args.sourceChainId).to.equal(31337);
      expect(event.args.destChainId).to.equal(AMOY_CHAIN_ID);
      expect(event.args.token).to.equal(await mockToken.getAddress());
      expect(event.args.amount).to.equal(amount);
      expect(event.args.sender).to.equal(user1.address);
      expect(event.args.recipient).to.equal(user2.address);

      // Check tokens were locked
      expect(await mockToken.balanceOf(await bridge.getAddress())).to.equal(amount);
    });

    it("Should revert if destination chain is the same as source", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          await mockToken.getAddress(),
          amount,
          31337, // Same as source
          user2.address,
          "0x"
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidDestinationChain");
    });

    it("Should revert if amount is zero", async function () {
      await expect(
        bridge.connect(user1).bridgeAsset(
          await mockToken.getAddress(),
          0,
          AMOY_CHAIN_ID,
          user2.address,
          "0x"
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidAmount");
    });

    it("Should revert if recipient is zero address", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          await mockToken.getAddress(),
          amount,
          AMOY_CHAIN_ID,
          ethers.ZeroAddress,
          "0x"
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidRecipient");
    });

    it("Should increment bridge nonce", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount * 2n);

      expect(await bridge.bridgeNonce()).to.equal(0);

      await bridge.connect(user1).bridgeAsset(
        await mockToken.getAddress(),
        amount,
        AMOY_CHAIN_ID,
        user2.address,
        "0x"
      );
      expect(await bridge.bridgeNonce()).to.equal(1);

      await bridge.connect(user1).bridgeAsset(
        await mockToken.getAddress(),
        amount,
        AMOY_CHAIN_ID,
        user2.address,
        "0x"
      );
      expect(await bridge.bridgeNonce()).to.equal(2);
    });
  });

  describe("BridgeAsset - Native Token", function () {
    it("Should lock native tokens and emit BridgeInitiated event", async function () {
      const amount = ethers.parseEther("1");

      const tx = await bridge.connect(user1).bridgeAsset(
        ethers.ZeroAddress, // native token
        amount,
        AMOY_CHAIN_ID,
        user2.address,
        "0x",
        { value: amount }
      );

      const receipt = await tx.wait();
      const event = receipt.logs.find(
        log => log.fragment && log.fragment.name === "BridgeInitiated"
      );

      expect(event).to.not.be.undefined;
      expect(event.args.token).to.equal(ethers.ZeroAddress);
      expect(event.args.amount).to.equal(amount);

      // Check native tokens were locked
      expect(await ethers.provider.getBalance(await bridge.getAddress())).to.equal(amount);
    });

    it("Should revert if msg.value does not match amount for native token", async function () {
      const amount = ethers.parseEther("1");

      await expect(
        bridge.connect(user1).bridgeAsset(
          ethers.ZeroAddress,
          amount,
          AMOY_CHAIN_ID,
          user2.address,
          "0x",
          { value: ethers.parseEther("0.5") } // Wrong value
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidAmount");
    });

    it("Should revert if msg.value is sent for ERC20 token", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          await mockToken.getAddress(),
          amount,
          AMOY_CHAIN_ID,
          user2.address,
          "0x",
          { value: ethers.parseEther("0.1") } // Should not send value for ERC20
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidAmount");
    });
  });

  describe("ExecuteTransaction (via Pelagos)", function () {
    const amount = ethers.parseUnits("500", 6);
    let bridgeId;

    beforeEach(async function () {
      // Add liquidity to bridge for claims
      await mockToken.mint(await bridge.getAddress(), ethers.parseUnits("5000", 6));

      // Generate a unique bridge ID
      bridgeId = ethers.keccak256(
        ethers.AbiCoder.defaultAbiCoder().encode(
          ["uint256", "uint256", "address", "uint256", "address", "uint256"],
          [11155111, 31337, await mockToken.getAddress(), amount, user2.address, Date.now()]
        )
      );
    });

    it("Should execute transaction via Pelagos and transfer tokens", async function () {
      // Build payload (160 bytes): bridgeId + sourceChainId + token + amount + recipient
      const tokenAddress = await mockToken.getAddress();

      // Token and recipient are LEFT-aligned (20 bytes + 12 zero bytes)
      const tokenPadded = tokenAddress + "000000000000000000000000";
      const recipientPadded = user2.address + "000000000000000000000000";

      const payload = ethers.concat([
        bridgeId,                                          // 32 bytes
        ethers.zeroPadValue(ethers.toBeHex(11155111), 32), // 32 bytes sourceChainId
        tokenPadded,                                       // 32 bytes token (left-aligned)
        ethers.zeroPadValue(ethers.toBeHex(amount), 32),   // 32 bytes amount
        recipientPadded                                    // 32 bytes recipient (left-aligned)
      ]);

      expect(payload.length).to.equal(322); // "0x" + 160*2 hex chars

      // Generate nonce hash
      const nonceHash = ethers.keccak256(payload);

      // Call via Pelagos
      await pelagos.processExternalTransaction(APP_CHAIN_ID, nonceHash, payload);

      // Check tokens were transferred
      expect(await mockToken.balanceOf(user2.address)).to.equal(amount);

      // Check bridge marked as claimed
      expect(await bridge.claimedBridges(bridgeId)).to.be.true;
    });

    it("Should revert if not called by Pelagos", async function () {
      const tokenAddress = await mockToken.getAddress();
      const tokenPadded = tokenAddress + "000000000000000000000000";
      const recipientPadded = user2.address + "000000000000000000000000";

      const payload = ethers.concat([
        bridgeId,
        ethers.zeroPadValue(ethers.toBeHex(11155111), 32),
        tokenPadded,
        ethers.zeroPadValue(ethers.toBeHex(amount), 32),
        recipientPadded
      ]);

      await expect(
        bridge.connect(user1).executeTransaction(payload)
      ).to.be.revertedWithCustomError(bridge, "Unauthorized");
    });

    it("Should revert if already claimed", async function () {
      const tokenAddress = await mockToken.getAddress();
      const tokenPadded = tokenAddress + "000000000000000000000000";
      const recipientPadded = user2.address + "000000000000000000000000";

      const payload = ethers.concat([
        bridgeId,
        ethers.zeroPadValue(ethers.toBeHex(11155111), 32),
        tokenPadded,
        ethers.zeroPadValue(ethers.toBeHex(amount), 32),
        recipientPadded
      ]);

      const nonceHash = ethers.keccak256(payload);
      await pelagos.processExternalTransaction(APP_CHAIN_ID, nonceHash, payload);

      // Try again with different nonce - Pelagos wraps Bridge revert with AppchainCallFailed
      const nonceHash2 = ethers.keccak256(ethers.concat([payload, "0x01"]));
      await expect(
        pelagos.processExternalTransaction(APP_CHAIN_ID, nonceHash2, payload)
      ).to.be.revertedWithCustomError(pelagos, "AppchainCallFailed");
    });

    it("Should revert if insufficient liquidity", async function () {
      const largeAmount = ethers.parseUnits("10000", 6);
      const newBridgeId = ethers.keccak256(
        ethers.AbiCoder.defaultAbiCoder().encode(
          ["uint256", "uint256", "address", "uint256", "address", "uint256"],
          [11155111, 31337, await mockToken.getAddress(), largeAmount, user2.address, Date.now() + 1000]
        )
      );

      const tokenAddress = await mockToken.getAddress();
      const tokenPadded = tokenAddress + "000000000000000000000000";
      const recipientPadded = user2.address + "000000000000000000000000";

      const payload = ethers.concat([
        newBridgeId,
        ethers.zeroPadValue(ethers.toBeHex(11155111), 32),
        tokenPadded,
        ethers.zeroPadValue(ethers.toBeHex(largeAmount), 32),
        recipientPadded
      ]);

      // Pelagos wraps Bridge revert with AppchainCallFailed
      const nonceHash = ethers.keccak256(payload);
      await expect(
        pelagos.processExternalTransaction(APP_CHAIN_ID, nonceHash, payload)
      ).to.be.revertedWithCustomError(pelagos, "AppchainCallFailed");
    });
  });

  describe("Native Token Claims", function () {
    it("Should execute native token claim via Pelagos", async function () {
      const amount = ethers.parseEther("1");

      // Add native liquidity
      await owner.sendTransaction({
        to: await bridge.getAddress(),
        value: ethers.parseEther("10")
      });

      const bridgeId = ethers.keccak256(
        ethers.AbiCoder.defaultAbiCoder().encode(
          ["uint256", "address", "uint256"],
          [11155111, user2.address, Date.now()]
        )
      );

      // Zero address for native token (left-aligned)
      const tokenPadded = ethers.ZeroAddress + "000000000000000000000000";
      const recipientPadded = user2.address + "000000000000000000000000";

      const payload = ethers.concat([
        bridgeId,
        ethers.zeroPadValue(ethers.toBeHex(11155111), 32),
        tokenPadded,
        ethers.zeroPadValue(ethers.toBeHex(amount), 32),
        recipientPadded
      ]);

      const nonceHash = ethers.keccak256(payload);
      const balanceBefore = await ethers.provider.getBalance(user2.address);

      await pelagos.processExternalTransaction(APP_CHAIN_ID, nonceHash, payload);

      const balanceAfter = await ethers.provider.getBalance(user2.address);
      expect(balanceAfter - balanceBefore).to.equal(amount);
    });
  });

  describe("Admin Functions", function () {
    it("Should allow owner to pause", async function () {
      await bridge.connect(owner).pause();
      expect(await bridge.paused()).to.be.true;

      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          await mockToken.getAddress(),
          amount,
          AMOY_CHAIN_ID,
          user2.address,
          "0x"
        )
      ).to.be.revertedWithCustomError(bridge, "EnforcedPause");
    });

    it("Should allow owner to unpause", async function () {
      await bridge.connect(owner).pause();
      await bridge.connect(owner).unpause();
      expect(await bridge.paused()).to.be.false;
    });

    it("Should allow owner to add liquidity", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(owner).approve(await bridge.getAddress(), amount);
      await mockToken.mint(owner.address, amount);

      await bridge.connect(owner).addLiquidity(await mockToken.getAddress(), amount);
      expect(await bridge.getBalance(await mockToken.getAddress())).to.equal(amount);
    });

    it("Should allow owner to add native liquidity", async function () {
      const amount = ethers.parseEther("1");
      await bridge.connect(owner).addLiquidity(ethers.ZeroAddress, amount, { value: amount });
      expect(await bridge.getBalance(ethers.ZeroAddress)).to.equal(amount);
    });

    it("Should allow owner to emergency withdraw", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      const balanceBefore = await mockToken.balanceOf(owner.address);
      await bridge.connect(owner).emergencyWithdraw(await mockToken.getAddress(), amount);
      const balanceAfter = await mockToken.balanceOf(owner.address);

      expect(balanceAfter - balanceBefore).to.equal(amount);
    });

    it("Should not allow non-owner to emergency withdraw", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).emergencyWithdraw(await mockToken.getAddress(), amount)
      ).to.be.revertedWithCustomError(bridge, "OwnableUnauthorizedAccount");
    });
  });

  describe("View Functions", function () {
    it("Should return bridge data", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      const tx = await bridge.connect(user1).bridgeAsset(
        await mockToken.getAddress(),
        amount,
        AMOY_CHAIN_ID,
        user2.address,
        "0x"
      );

      const receipt = await tx.wait();
      const event = receipt.logs.find(
        log => log.fragment && log.fragment.name === "BridgeInitiated"
      );
      const bridgeId = event.args.bridgeId;

      const bridgeData = await bridge.getBridge(bridgeId);
      expect(bridgeData.sourceChainId).to.equal(31337);
      expect(bridgeData.destChainId).to.equal(AMOY_CHAIN_ID);
      expect(bridgeData.token).to.equal(await mockToken.getAddress());
      expect(bridgeData.amount).to.equal(amount);
      expect(bridgeData.sender).to.equal(user1.address);
      expect(bridgeData.recipient).to.equal(user2.address);
      expect(bridgeData.claimed).to.be.false;
    });

    it("Should return correct balance", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      expect(await bridge.getBalance(await mockToken.getAddress())).to.equal(amount);
    });
  });
});

describe("Pelagos", function () {
  let pelagos;
  let owner;
  let user1;

  beforeEach(async function () {
    [owner, user1] = await ethers.getSigners();

    const Pelagos = await ethers.getContractFactory("Pelagos");
    pelagos = await Pelagos.deploy();
    await pelagos.waitForDeployment();
  });

  describe("Deployment", function () {
    it("Should set the correct owner", async function () {
      expect(await pelagos.owner()).to.equal(owner.address);
    });

    it("Should start with zero processed transactions", async function () {
      expect(await pelagos.totalProcessedTransactions()).to.equal(0);
    });
  });

  describe("Ownership", function () {
    it("Should allow owner to transfer ownership", async function () {
      await pelagos.transferOwnership(user1.address);
      expect(await pelagos.owner()).to.equal(user1.address);
    });

    it("Should not allow non-owner to transfer ownership", async function () {
      await expect(
        pelagos.connect(user1).transferOwnership(user1.address)
      ).to.be.revertedWithCustomError(pelagos, "CallerNotOwner");
    });

    it("Should not allow transfer to zero address", async function () {
      await expect(
        pelagos.transferOwnership(ethers.ZeroAddress)
      ).to.be.revertedWithCustomError(pelagos, "InvalidOwner");
    });
  });

  describe("Appchain Registration", function () {
    it("Should register appchain contract", async function () {
      await pelagos.registerAppchainContract(1604, user1.address);
      expect(await pelagos.appchainContracts(1604)).to.equal(user1.address);
    });

    it("Should emit event on registration", async function () {
      await expect(pelagos.registerAppchainContract(1604, user1.address))
        .to.emit(pelagos, "AppchainRegistered")
        .withArgs(1604, user1.address);
    });

    it("Should unregister appchain contract", async function () {
      await pelagos.registerAppchainContract(1604, user1.address);
      await pelagos.unregisterAppchainContract(1604);
      expect(await pelagos.appchainContracts(1604)).to.equal(ethers.ZeroAddress);
    });

    it("Should not allow non-owner to register", async function () {
      await expect(
        pelagos.connect(user1).registerAppchainContract(1604, user1.address)
      ).to.be.revertedWithCustomError(pelagos, "CallerNotOwner");
    });
  });

  describe("Process External Transaction", function () {
    it("Should reject empty payload", async function () {
      const nonceHash = ethers.keccak256(ethers.toUtf8Bytes("test"));
      await expect(
        pelagos.processExternalTransaction(1604, nonceHash, "0x")
      ).to.be.revertedWithCustomError(pelagos, "EmptyPayload");
    });

    it("Should reject zero appchain ID", async function () {
      const nonceHash = ethers.keccak256(ethers.toUtf8Bytes("test"));
      await expect(
        pelagos.processExternalTransaction(0, nonceHash, "0x1234")
      ).to.be.revertedWithCustomError(pelagos, "InvalidAppChainId");
    });

    it("Should reject duplicate nonce hash", async function () {
      const nonceHash = ethers.keccak256(ethers.toUtf8Bytes("test"));

      // First call succeeds (emits AppchainNotRegistered since no contract registered)
      await pelagos.processExternalTransaction(1604, nonceHash, "0x1234");

      // Second call with same nonce should fail
      await expect(
        pelagos.processExternalTransaction(1604, nonceHash, "0x1234")
      ).to.be.revertedWithCustomError(pelagos, "TransactionAlreadyProcessed");
    });

    it("Should emit AppchainNotRegistered when no contract registered", async function () {
      const nonceHash = ethers.keccak256(ethers.toUtf8Bytes("test"));
      await expect(pelagos.processExternalTransaction(1604, nonceHash, "0x1234"))
        .to.emit(pelagos, "AppchainNotRegistered")
        .withArgs(1604, "0x1234");
    });

    it("Should increment total processed transactions", async function () {
      const nonceHash1 = ethers.keccak256(ethers.toUtf8Bytes("test1"));
      const nonceHash2 = ethers.keccak256(ethers.toUtf8Bytes("test2"));

      await pelagos.processExternalTransaction(1604, nonceHash1, "0x1234");
      expect(await pelagos.totalProcessedTransactions()).to.equal(1);

      await pelagos.processExternalTransaction(1604, nonceHash2, "0x5678");
      expect(await pelagos.totalProcessedTransactions()).to.equal(2);
    });
  });
});
