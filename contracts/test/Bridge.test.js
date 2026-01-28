const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("Bridge", function () {
  let bridge;
  let mockToken;
  let owner;
  let validator;
  let user1;
  let user2;

  const SEPOLIA_CHAIN_ID = 11155111;
  const AMOY_CHAIN_ID = 80002;

  beforeEach(async function () {
    [owner, validator, user1, user2] = await ethers.getSigners();

    // Deploy mock token
    const MockERC20 = await ethers.getContractFactory("MockERC20");
    mockToken = await MockERC20.deploy("Test USDT", "USDT", 6);
    await mockToken.waitForDeployment();

    // Mint tokens to user1
    await mockToken.mint(user1.address, ethers.parseUnits("10000", 6));

    // Deploy bridge
    const Bridge = await ethers.getContractFactory("Bridge");
    bridge = await Bridge.deploy(validator.address);
    await bridge.waitForDeployment();
  });

  describe("Deployment", function () {
    it("Should set the correct validator", async function () {
      expect(await bridge.validator()).to.equal(validator.address);
    });

    it("Should set the correct chainId", async function () {
      expect(await bridge.chainId()).to.equal(31337); // Hardhat network
    });

    it("Should set the correct owner", async function () {
      expect(await bridge.owner()).to.equal(owner.address);
    });
  });

  describe("BridgeAsset", function () {
    it("Should lock tokens and emit BridgeInitiated event", async function () {
      const amount = ethers.parseUnits("1000", 6);

      // Approve bridge to spend tokens
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      // Bridge tokens
      const tx = await bridge.connect(user1).bridgeAsset(
        AMOY_CHAIN_ID,
        user2.address,
        await mockToken.getAddress(),
        amount
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
          31337, // Same as source
          user2.address,
          await mockToken.getAddress(),
          amount
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidDestinationChain");
    });

    it("Should revert if amount is zero", async function () {
      await expect(
        bridge.connect(user1).bridgeAsset(
          AMOY_CHAIN_ID,
          user2.address,
          await mockToken.getAddress(),
          0
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidAmount");
    });

    it("Should revert if recipient is zero address", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          AMOY_CHAIN_ID,
          ethers.ZeroAddress,
          await mockToken.getAddress(),
          amount
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidRecipient");
    });

    it("Should revert if token is zero address", async function () {
      const amount = ethers.parseUnits("1000", 6);

      await expect(
        bridge.connect(user1).bridgeAsset(
          AMOY_CHAIN_ID,
          user2.address,
          ethers.ZeroAddress,
          amount
        )
      ).to.be.revertedWithCustomError(bridge, "InvalidToken");
    });

    it("Should increment bridge nonce", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount * 2n);

      expect(await bridge.bridgeNonce()).to.equal(0);

      await bridge.connect(user1).bridgeAsset(
        AMOY_CHAIN_ID,
        user2.address,
        await mockToken.getAddress(),
        amount
      );
      expect(await bridge.bridgeNonce()).to.equal(1);

      await bridge.connect(user1).bridgeAsset(
        AMOY_CHAIN_ID,
        user2.address,
        await mockToken.getAddress(),
        amount
      );
      expect(await bridge.bridgeNonce()).to.equal(2);
    });
  });

  describe("ClaimAsset", function () {
    let bridgeId;
    const amount = ethers.parseUnits("500", 6);

    beforeEach(async function () {
      // Add liquidity to bridge for claims
      await mockToken.mint(await bridge.getAddress(), ethers.parseUnits("5000", 6));

      // Generate a unique bridge ID
      bridgeId = ethers.keccak256(
        ethers.AbiCoder.defaultAbiCoder().encode(
          ["uint256", "uint256", "address", "uint256", "address", "uint256"],
          [SEPOLIA_CHAIN_ID, 31337, await mockToken.getAddress(), amount, user2.address, Date.now()]
        )
      );
    });

    it("Should claim tokens and emit AssetClaimed event", async function () {
      const tx = await bridge.connect(validator).claimAsset(
        bridgeId,
        SEPOLIA_CHAIN_ID,
        await mockToken.getAddress(),
        amount,
        user2.address
      );

      const receipt = await tx.wait();
      const event = receipt.logs.find(
        log => log.fragment && log.fragment.name === "AssetClaimed"
      );

      expect(event).to.not.be.undefined;
      expect(event.args.bridgeId).to.equal(bridgeId);
      expect(event.args.recipient).to.equal(user2.address);
      expect(event.args.token).to.equal(await mockToken.getAddress());
      expect(event.args.amount).to.equal(amount);

      // Check tokens were transferred
      expect(await mockToken.balanceOf(user2.address)).to.equal(amount);
    });

    it("Should mark bridge as claimed", async function () {
      await bridge.connect(validator).claimAsset(
        bridgeId,
        SEPOLIA_CHAIN_ID,
        await mockToken.getAddress(),
        amount,
        user2.address
      );

      expect(await bridge.isClaimed(bridgeId)).to.be.true;
    });

    it("Should revert if already claimed", async function () {
      await bridge.connect(validator).claimAsset(
        bridgeId,
        SEPOLIA_CHAIN_ID,
        await mockToken.getAddress(),
        amount,
        user2.address
      );

      await expect(
        bridge.connect(validator).claimAsset(
          bridgeId,
          SEPOLIA_CHAIN_ID,
          await mockToken.getAddress(),
          amount,
          user2.address
        )
      ).to.be.revertedWithCustomError(bridge, "BridgeAlreadyClaimed");
    });

    it("Should revert if not called by validator", async function () {
      await expect(
        bridge.connect(user1).claimAsset(
          bridgeId,
          SEPOLIA_CHAIN_ID,
          await mockToken.getAddress(),
          amount,
          user2.address
        )
      ).to.be.revertedWithCustomError(bridge, "Unauthorized");
    });

    it("Should revert if insufficient liquidity", async function () {
      const largeAmount = ethers.parseUnits("10000", 6);
      const newBridgeId = ethers.keccak256(
        ethers.AbiCoder.defaultAbiCoder().encode(
          ["uint256", "uint256", "address", "uint256", "address", "uint256"],
          [SEPOLIA_CHAIN_ID, 31337, await mockToken.getAddress(), largeAmount, user2.address, Date.now() + 1000]
        )
      );

      await expect(
        bridge.connect(validator).claimAsset(
          newBridgeId,
          SEPOLIA_CHAIN_ID,
          await mockToken.getAddress(),
          largeAmount,
          user2.address
        )
      ).to.be.revertedWithCustomError(bridge, "InsufficientLiquidity");
    });
  });

  describe("Liquidity Management", function () {
    it("Should allow anyone to add liquidity", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).addLiquidity(await mockToken.getAddress(), amount)
      ).to.emit(bridge, "LiquidityAdded")
        .withArgs(await mockToken.getAddress(), amount, user1.address);

      expect(await bridge.getLiquidity(await mockToken.getAddress())).to.equal(amount);
    });

    it("Should allow owner to remove liquidity", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(owner).removeLiquidity(
          await mockToken.getAddress(),
          amount,
          user1.address
        )
      ).to.emit(bridge, "LiquidityRemoved")
        .withArgs(await mockToken.getAddress(), amount, user1.address);

      expect(await mockToken.balanceOf(user1.address)).to.equal(
        ethers.parseUnits("10000", 6) + amount
      );
    });

    it("Should not allow non-owner to remove liquidity", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).removeLiquidity(
          await mockToken.getAddress(),
          amount,
          user1.address
        )
      ).to.be.revertedWithCustomError(bridge, "OwnableUnauthorizedAccount");
    });
  });

  describe("Admin Functions", function () {
    it("Should allow owner to update validator", async function () {
      await expect(
        bridge.connect(owner).updateValidator(user1.address)
      ).to.emit(bridge, "ValidatorUpdated")
        .withArgs(validator.address, user1.address);

      expect(await bridge.validator()).to.equal(user1.address);
    });

    it("Should allow owner to pause", async function () {
      await bridge.connect(owner).pause();
      expect(await bridge.paused()).to.be.true;

      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(user1).bridgeAsset(
          AMOY_CHAIN_ID,
          user2.address,
          await mockToken.getAddress(),
          amount
        )
      ).to.be.revertedWithCustomError(bridge, "EnforcedPause");
    });

    it("Should allow owner to unpause", async function () {
      await bridge.connect(owner).pause();
      await bridge.connect(owner).unpause();
      expect(await bridge.paused()).to.be.false;
    });

    it("Should allow emergency withdrawal when paused", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      await bridge.connect(owner).pause();
      await bridge.connect(owner).emergencyWithdraw(
        await mockToken.getAddress(),
        amount,
        user1.address
      );

      expect(await mockToken.balanceOf(user1.address)).to.equal(
        ethers.parseUnits("10000", 6) + amount
      );
    });

    it("Should not allow emergency withdrawal when not paused", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      await expect(
        bridge.connect(owner).emergencyWithdraw(
          await mockToken.getAddress(),
          amount,
          user1.address
        )
      ).to.be.revertedWithCustomError(bridge, "ExpectedPause");
    });
  });

  describe("View Functions", function () {
    it("Should return bridge data", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.connect(user1).approve(await bridge.getAddress(), amount);

      const tx = await bridge.connect(user1).bridgeAsset(
        AMOY_CHAIN_ID,
        user2.address,
        await mockToken.getAddress(),
        amount
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

    it("Should return correct liquidity", async function () {
      const amount = ethers.parseUnits("1000", 6);
      await mockToken.mint(await bridge.getAddress(), amount);

      expect(await bridge.getLiquidity(await mockToken.getAddress())).to.equal(amount);
    });
  });
});
