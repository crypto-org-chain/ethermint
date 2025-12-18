pragma solidity ^0.8.10;

/**
 * @title GasConsumerTryCatch
 * @notice A contract to test try-catch behavior with high gas consumption using a single contract
 */
contract GasConsumerTryCatch {
    mapping(uint256 => uint256) public data;
    uint256 public totalWrites;
    uint256 public lastResult;
    uint256 public callCount;

    event TrySuccess(uint256 result, uint256 gasUsed);
    event TryCatchFailed(string reason, uint256 gasUsed);
    event TryCatchFailedBytes(bytes reason, uint256 gasUsed);

    error GasConsumerReverted(uint256 iterationsCompleted);

    /**
     * @notice Consumes gas by writing to storage.
     * Must be external to be called via this.consumeGas() in try-catch.
     * @param iterations Number of storage writes (~20,000 gas each)
     * @param shouldRevert If true, reverts after consuming gas
     * @return The total number of writes performed
     */
    function consumeGas(uint256 iterations, bool shouldRevert) external returns (uint256) {
        uint256 startValue = totalWrites;

        // Each SSTORE costs ~20,000 gas for a new slot (cold access)
        // To consume ~400,000 gas, we need about 20 iterations
        for (uint256 i = 0; i < iterations; i++) {
            data[startValue + i] = block.timestamp + i;
            totalWrites++;
        }

        if (shouldRevert) {
            revert GasConsumerReverted(iterations);
        }

        return totalWrites;
    }

    /**
     * @notice Calls the gas-consuming function with try-catch
     * @param iterations Number of storage write iterations
     * @param shouldRevert If true, the try block will revert after consuming gas
     */
    function callWithTryCatch(uint256 iterations, bool shouldRevert) external returns (bool success) {
        uint256 gasBefore = gasleft();
        callCount++;

        // using "this" to make an external call, enabling try-catch
        try this.consumeGas(iterations, shouldRevert) returns (uint256 result) {
            uint256 gasUsed = gasBefore - gasleft();
            lastResult = result;
            emit TrySuccess(result, gasUsed);
            return true;
        } catch Error(string memory reason) {
            uint256 gasUsed = gasBefore - gasleft();
            emit TryCatchFailed(reason, gasUsed);
            return false;
        } catch (bytes memory reason) {
            uint256 gasUsed = gasBefore - gasleft();
            emit TryCatchFailedBytes(reason, gasUsed);
            return false;
        }
    }
}