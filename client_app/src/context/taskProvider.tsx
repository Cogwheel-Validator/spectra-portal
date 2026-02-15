"use client";

import type { EncodeObject } from "@cosmjs/proto-signing";
import {
    createContext,
    type ReactNode,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useRef,
    useState,
} from "react";
import type { ClientChain, ClientConfig } from "@/components/modules/tomlTypes";
import useIbcTracking from "@/hooks/useIbcTracking";
import useTransactionConstructor from "@/hooks/useTransactionConstr";
import clientLogger from "@/lib/clientLogger";
import type {
    SwapAmountInRoute,
    SwapAmountInSplitRoute,
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";
import type {
    BrokerSwapRoute,
    FindPathResponse,
    WasmMsg,
} from "@/lib/generated/pathfinder/pathfinder_route_pb";
import {
    AddTransactionToDb,
    ConnectToDb,
    type TransactionRecord,
    UpdateTransactionStatus,
} from "@/lib/indexDb/dbManager";
import { humanToBaseUnits } from "@/lib/utils";
import {
    extractChainPath,
    generateStepsFromResponse,
    type TransferMode,
    type TransferStep,
    useTransfer,
} from "./transferContext";
import { useWallet } from "./walletContext";

// ============================================================================
// Types
// ============================================================================

export const TaskTypes = {
    // IBC transfers
    IBC_TRANSFER: "ibc-basic-transfer",
    IBC_HOOKS_TRANSFER: "ibc-hooks-transfer",
    // Osmosis swaps
    OSMO_SWAP: "osmosis-swap",
    OSMO_SPLIT_ROUTE_SWAP: "osmosis-split-route-swap",
    WASM_EXECUTION: "wasm-execution",
    MULTI_HOP: "multi-hop",
    MULTI_HOP_SWAP: "multi-hop + swap",
    // Indexed DB related tasks
    INSERT_TRANSACTION: "insert-transaction",
    UPDATE_TRANSACTION: "update-transaction",
} as const;

export type TaskType = (typeof TaskTypes)[keyof typeof TaskTypes];

/**
 * Execution result from a task
 */
export interface TaskExecutionResult {
    success: boolean;
    txHash?: string;
    error?: string;
    isRetryable?: boolean;
}

/**
 * Task context interface
 */
interface TaskContextType {
    // Database connection
    db: IDBDatabase | null;
    isDbReady: boolean;

    // Execution methods
    executeTransfer: (
        response: FindPathResponse,
        mode: TransferMode,
        config: ClientConfig,
    ) => Promise<void>;
    executeStep: (step: TransferStep, config: ClientConfig) => Promise<TaskExecutionResult>;
    retryStep: (stepIndex: number, config: ClientConfig) => Promise<TaskExecutionResult>;

    // State
    isExecuting: boolean;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export const useTaskProvider = () => {
    const context = useContext(TaskContext);
    if (!context) {
        throw new Error("useTaskProvider must be used within TaskProvider");
    }
    return context;
};

// ============================================================================
// Provider
// ============================================================================

export const TaskProvider = ({ children }: { children: ReactNode }) => {
    const [db, setDb] = useState<IDBDatabase | null>(null);
    const [isDbReady, setIsDbReady] = useState(false);
    const [isExecuting, setIsExecuting] = useState(false);
    const executionLock = useRef(false);

    const { sendTransaction, getAddress } = useWallet();
    const transfer = useTransfer();
    const { trackIbcTransfer, trackSmartTransferMultiHop } = useIbcTracking();
    const {
        createIbcTransferMessage,
        createSwapMessage,
        createSplitRouteSwapMessage,
        createWasmExecutionMessage,
    } = useTransactionConstructor();

    // Initialize database connection
    useEffect(() => {
        let mounted = true;

        const initDb = async () => {
            try {
                const database = await ConnectToDb();
                if (mounted) {
                    setDb(database);
                    setIsDbReady(true);
                }
            } catch (error) {
                clientLogger.error("Failed to connect to IndexedDB:", error);
            }
        };

        initDb();

        return () => {
            mounted = false;
        };
    }, []);

    /**
     * Execute a single transfer step
     */
    const executeStep = useCallback(
        async (step: TransferStep, config: ClientConfig): Promise<TaskExecutionResult> => {
            const chainConfig = config.chains.find((c) => c.id === step.fromChain);
            if (!chainConfig) {
                return {
                    success: false,
                    error: `Chain config not found for ${step.fromChain}`,
                };
            }

            const senderAddress = getAddress(step.fromChain);
            if (!senderAddress) {
                return {
                    success: false,
                    error: `Not connected to ${step.fromChain}`,
                    isRetryable: true,
                };
            }

            try {
                let message: EncodeObject;
                let tx_memo: string;

                switch (step.type) {
                    case "ibc_transfer":
                    case "multi-hop": {
                        const receiverAddress =
                            getAddress(step.toChain) || transfer.state.receiverAddress;
                        if (!receiverAddress) {
                            return {
                                success: false,
                                error: `Receiver address not available for ${step.toChain}`,
                                isRetryable: true,
                            };
                        }

                        message = createIbcTransferMessage(
                            senderAddress,
                            receiverAddress,
                            step.metadata?.channel || "",
                            step.metadata?.port || "transfer",
                            {
                                amount: step.metadata?.amount || "0",
                                denom: step.metadata?.denom || "",
                            },
                            step.metadata?.memo || "",
                        );
                        tx_memo = "Spectra IBC transfer";
                        break;
                    }

                    case "multi-hop + swap": {
                        // Multi-hop + swap: IBC transfer with a wasm/PFM memo
                        // The memo contains instructions for the broker chain to execute swap + forward
                        const brokerRouteMs = transfer.state.pathfinderResponse?.route
                            .value as BrokerSwapRoute;
                        const executionMs = brokerRouteMs?.execution;

                        // The IBC receiver comes from the pathfinder execution data
                        // For 1 inbound leg: the receiver is the contract on broker chain
                        // For 2 inbound legs: the receiver is the user's address on intermediate chain (PFM forwards)
                        const receiverMs =
                            executionMs?.ibcReceiver ||
                            getAddress(step.toChain) ||
                            transfer.state.receiverAddress;

                        if (!receiverMs) {
                            return {
                                success: false,
                                error: `Receiver address not available for ${step.toChain}`,
                                isRetryable: true,
                            };
                        }

                        message = createIbcTransferMessage(
                            senderAddress,
                            receiverMs,
                            step.metadata?.channel || "",
                            step.metadata?.port || "transfer",
                            {
                                amount: step.metadata?.amount || "0",
                                denom: step.metadata?.denom || "",
                            },
                            step.metadata?.memo || "",
                        );
                        tx_memo = "Spectra IBC multi-hop + swap";
                        break;
                    }

                    case "wasm-execution": {
                        // WASM execution on the broker chain (no inbound IBC legs)
                        // e.g., Swap + IBC transfer from Osmosis, or Swap + multi-hop from Osmosis
                        const brokerRouteWasm = transfer.state.pathfinderResponse?.route
                            .value as BrokerSwapRoute;
                        const executionWasm = brokerRouteWasm?.execution;

                        if (
                            !executionWasm?.smartContractData?.contract ||
                            !executionWasm?.smartContractData?.msg
                        ) {
                            return {
                                success: false,
                                error: "Smart contract data not found in execution",
                            };
                        }

                        // Include funds: the token being sent to the contract for swapping
                        const swapDataWasm = brokerRouteWasm?.swap;
                        const funds =
                            swapDataWasm?.tokenIn?.chainDenom && swapDataWasm?.amountIn
                                ? [
                                      {
                                          denom: swapDataWasm.tokenIn.chainDenom,
                                          amount: swapDataWasm.amountIn,
                                      },
                                  ]
                                : [];

                        message = createWasmExecutionMessage(
                            senderAddress,
                            executionWasm.smartContractData.contract,
                            executionWasm.smartContractData.msg as WasmMsg,
                            funds,
                        );
                        tx_memo = "Spectra WASM execution";
                        break;
                    }

                    case "swap": {
                        // Osmosis swap - can be single route or split route
                        const brokerRoute = transfer.state.pathfinderResponse?.route
                            .value as BrokerSwapRoute;
                        const swapData = brokerRoute?.swap;
                        const osmosisRouteData = swapData?.routeData;

                        if (!swapData || osmosisRouteData?.case !== "osmosisRouteData") {
                            return { success: false, error: "Swap route data not found" };
                        }

                        const osmosisRoutes = osmosisRouteData.value.routes;

                        // Check if this is a split route (multiple routes) or single route
                        if (osmosisRoutes.length > 1) {
                            // Split route swap
                            const splitRoutes: SwapAmountInSplitRoute[] = osmosisRoutes.map(
                                (route) => ({
                                    pools: route.pools.map((pool) => ({
                                        poolId: Number(pool.id),
                                        tokenOutDenom: pool.tokenOutDenom,
                                    })),
                                    tokenInAmount: route.inAmount,
                                }),
                            );

                            message = createSplitRouteSwapMessage(
                                senderAddress,
                                swapData.tokenIn?.chainDenom || "",
                                splitRoutes,
                                swapData.amountOut, // Use expected output as minimum
                            );
                        } else {
                            // Single route swap
                            const routes: SwapAmountInRoute[] = osmosisRoutes.flatMap((route) =>
                                route.pools.map((pool) => ({
                                    poolId: Number(pool.id),
                                    tokenOutDenom: pool.tokenOutDenom,
                                })),
                            );

                            message = createSwapMessage(
                                senderAddress,
                                {
                                    amount: swapData.amountIn,
                                    denom: swapData.tokenIn?.chainDenom || "",
                                },
                                routes,
                                swapData.amountOut, // Use expected output as minimum
                            );
                        }
                        tx_memo = "Spectra Osmosis Swap";
                        break;
                    }

                    default:
                        return { success: false, error: `Unknown step type: ${step.type}` };
                }

                const result = await sendTransaction(step.fromChain, [message], {
                    memo: tx_memo,
                    chainConfig,
                });

                if (result.code === 0) {
                    return { success: true, txHash: result.transactionHash };
                } else {
                    return {
                        success: false,
                        error: `Transaction failed with code ${result.code}`,
                        txHash: result.transactionHash,
                        isRetryable: true,
                    };
                }
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : "Unknown error";
                const isRetryable =
                    errorMessage.toLowerCase().includes("out of gas") ||
                    errorMessage.toLowerCase().includes("insufficient") ||
                    errorMessage.toLowerCase().includes("timeout") ||
                    errorMessage.toLowerCase().includes("rejected");

                return { success: false, error: errorMessage, isRetryable };
            }
        },
        [
            getAddress,
            sendTransaction,
            transfer.state.pathfinderResponse,
            transfer.state.receiverAddress,
            createIbcTransferMessage,
            createSwapMessage,
            createSplitRouteSwapMessage,
            createWasmExecutionMessage,
        ],
    );

    /**
     * Execute steps with IBC tracking and progress updates
     * This is the core execution logic shared by both executeTransfer and retryStep
     */
    const executeStepsWithTracking = useCallback(
        async (
            steps: TransferStep[],
            startIndex: number,
            config: ClientConfig,
            mode: TransferMode,
            chainPath: string[],
            dbRecordId: number | null,
        ): Promise<TaskExecutionResult> => {
            if (!db) {
                return { success: false, error: "Database not ready" };
            }

            for (let i = startIndex; i < steps.length; i++) {
                const step = steps[i];

                // Skip already completed steps
                if (step.status === "completed") {
                    continue;
                }

                transfer.updateStep(i, { status: "signing" });

                const result = await executeStep(step, config);

                if (result.success && result.txHash) {
                    // Check if this is an IBC transfer that needs tracking
                    const isIbcStep =
                        step.type === "ibc_transfer" ||
                        step.type === "multi-hop" ||
                        step.type === "wasm-execution" ||
                        step.type === "multi-hop + swap";

                    const isMultiHopStep =
                        (step.type === "multi-hop + swap" ||
                            step.type === "multi-hop" ||
                            step.type === "wasm-execution") &&
                        mode === "smart";

                    if (isIbcStep && !isMultiHopStep) {
                        // Update to "confirming" status while we track
                        transfer.updateStep(i, {
                            status: "confirming",
                            txHash: result.txHash,
                        });

                        // Get destination chain config for tracking
                        const destChain = config.chains.find((c) => c.id === step.toChain) as
                            | ClientChain
                            | undefined;

                        if (destChain) {
                            // Track the IBC transfer on destination chain
                            const trackingResult = await trackIbcTransfer(
                                step.fromChain,
                                result.txHash,
                                destChain,
                                {
                                    maxAttempts: 60,
                                    pollInterval: 10000,
                                    timeout: 10 * 60 * 1000,
                                    onProgress: (attempt, max) => {
                                        clientLogger.info(
                                            `Tracking IBC transfer: attempt ${attempt}/${max}`,
                                        );
                                    },
                                },
                            );

                            clientLogger.info("trackingResult", trackingResult);

                            if (!trackingResult.success) {
                                const error = trackingResult.error || "IBC tracking inconclusive";
                                transfer.failTransfer(error);

                                if (dbRecordId != null) {
                                    await UpdateTransactionStatus(db, {
                                        id: dbRecordId,
                                        status: "failed",
                                        error,
                                    });
                                }

                                return { success: false, error };
                            }
                        }
                    }

                    if (isMultiHopStep) {
                        // Single-step multi-hop (wasm/PFM): track across all chains
                        transfer.updateStep(i, {
                            status: "confirming",
                            txHash: result.txHash,
                        });
                        const path = step.transferOrder ?? chainPath;
                        const totalChains = path.length;
                        if (totalChains >= 2) {
                            transfer.setMultiHopProgress(0, totalChains);
                            if (dbRecordId != null) {
                                await UpdateTransactionStatus(db, {
                                    id: dbRecordId,
                                    multiHopProgress: 0,
                                    multiHopTotalChains: totalChains,
                                });
                            }
                            const trackingResult = await trackSmartTransferMultiHop(
                                step.fromChain,
                                result.txHash,
                                path,
                                config.chains as ClientChain[],
                                {
                                    maxAttempts: 60,
                                    pollInterval: 10000,
                                    timeout: 10 * 60 * 1000,
                                    onHopComplete: (completedChains, total) => {
                                        const pct =
                                            total > 0
                                                ? Math.round((completedChains / total) * 100)
                                                : 0;
                                        if (dbRecordId != null) {
                                            UpdateTransactionStatus(db, {
                                                id: dbRecordId,
                                                multiHopProgress: pct,
                                                multiHopTotalChains: total,
                                            }).catch((err) =>
                                                clientLogger.warn(
                                                    "Update multiHopProgress failed",
                                                    err,
                                                ),
                                            );
                                        }
                                        transfer.setMultiHopProgress(pct, total);
                                    },
                                },
                            );
                            transfer.setMultiHopProgress(null);
                            if (!trackingResult.success) {
                                const error = trackingResult.error || "IBC tracking inconclusive";
                                transfer.failTransfer(error);

                                if (dbRecordId != null) {
                                    await UpdateTransactionStatus(db, {
                                        id: dbRecordId,
                                        status: "failed",
                                        error,
                                    });
                                }

                                return { success: false, error };
                            }
                        }
                    }

                    // Mark step as completed
                    transfer.updateStep(i, {
                        status: "completed",
                        txHash: result.txHash,
                    });

                    // Update database
                    if (dbRecordId != null) {
                        await UpdateTransactionStatus(db, {
                            id: dbRecordId,
                            currentStep: i + 1,
                        });
                    }

                    // If not the last step, advance
                    if (i < steps.length - 1) {
                        transfer.advanceStep();
                    }
                } else {
                    transfer.updateStep(i, {
                        status: "failed",
                        error: result.error,
                    });

                    if (dbRecordId != null) {
                        await UpdateTransactionStatus(db, {
                            id: dbRecordId,
                            status: "failed",
                            error: result.error || "Unknown error",
                        });
                    }

                    transfer.failTransfer(result.error || "Step execution failed");

                    return {
                        success: false,
                        error: result.error,
                        isRetryable: result.isRetryable,
                    };
                }
            }

            return { success: true };
        },
        [db, transfer, executeStep, trackIbcTransfer, trackSmartTransferMultiHop],
    );

    /**
     * Execute the complete transfer flow
     */
    const executeTransfer = useCallback(
        async (
            response: FindPathResponse,
            mode: TransferMode,
            config: ClientConfig,
        ): Promise<void> => {
            if (!db || !response.success) {
                transfer.failTransfer("Invalid transfer state");
                return;
            }

            if (executionLock.current) {
                clientLogger.warn("Transfer already executing, ignoring duplicate call");
                return;
            }

            executionLock.current = true;
            setIsExecuting(true);

            try {
                // Generate steps from the pathfinder response
                const steps = generateStepsFromResponse(response, mode);
                const chainPath = extractChainPath(response);

                if (steps.length === 0) {
                    transfer.failTransfer("No steps generated from route");
                    return;
                }

                // Create transaction record in IndexedDB
                // Convert human-readable amount to base units for consistent storage
                const fromTokenDecimals = transfer.state.fromToken?.decimals ?? 6;
                const amountInBaseUnits = humanToBaseUnits(
                    transfer.state.amount,
                    fromTokenDecimals,
                );

                const transactionRecord: Omit<TransactionRecord, "id"> = {
                    timestamp: new Date().toISOString(),
                    fromChainId: transfer.state.fromChainId,
                    fromChainAddress: transfer.state.senderAddress,
                    toChainId: transfer.state.toChainId,
                    toChainAddress: transfer.state.receiverAddress,
                    tokenIn: transfer.state.fromToken?.denom || "",
                    tokenOut:
                        transfer.state.toToken?.denom || transfer.state.fromToken?.denom || "",
                    amountIn: amountInBaseUnits,
                    amountOut: getExpectedOutput(response),
                    typeOfTransfer: mode,
                    status: "in-progress",
                    totalSteps: steps.length,
                    currentStep: 0,
                    trajectory: chainPath.length > 2 ? chainPath.slice(1, -1) : null,
                    error: null,
                    swapInvolved: response.route.case === "brokerSwap",
                };

                const dbRecordId = await AddTransactionToDb(db, transactionRecord);

                // Start execution with generated steps
                transfer.startExecuting(steps, dbRecordId, chainPath);

                // Execute all steps with tracking
                const result = await executeStepsWithTracking(
                    steps,
                    0,
                    config,
                    mode,
                    chainPath,
                    dbRecordId,
                );

                if (result.success) {
                    // All steps completed
                    await UpdateTransactionStatus(db, {
                        id: dbRecordId,
                        status: "success",
                        currentStep: steps.length,
                    });

                    transfer.completeTransfer();
                }
                // If not successful, error handling is already done in executeStepsWithTracking
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : "Unknown error";
                transfer.failTransfer(errorMessage);
            } finally {
                executionLock.current = false;
                setIsExecuting(false);
            }
        },
        [db, transfer, executeStepsWithTracking],
    );

    /**
     * Retry a failed step.
     *
     * When a step is retried we:
     * - Move the overall transfer phase out of "failed" back to "executing"
     * - Mark the IndexedDB transaction as "in-progress" again
     * - If the retry ultimately succeeds and all steps are completed, mark the
     *   transfer (and DB record) as successful instead of leaving it as failed.
     * - Continue executing remaining steps if any exist
     */
    const retryStep = useCallback(
        async (stepIndex: number, config: ClientConfig): Promise<TaskExecutionResult> => {
            if (!db) {
                return { success: false, error: "Database not ready" };
            }

            const step = transfer.state.steps[stepIndex];
            if (!step) {
                return { success: false, error: "Step not found" };
            }

            // Snapshot current steps for completion checks later
            const existingSteps = transfer.state.steps;
            const dbRecordId = transfer.state.dbRecordId;
            const chainPath = transfer.state.chainPath;
            const mode = transfer.state.mode;

            try {
                // Move transfer back into an executing state and clear top-level error
                if (dbRecordId != null) {
                    transfer.resumeTransfer(existingSteps, stepIndex, dbRecordId, chainPath);
                }

                // Mark the persisted transaction as in-progress again
                if (dbRecordId != null) {
                    await UpdateTransactionStatus(db, {
                        id: dbRecordId,
                        status: "in-progress",
                        error: null,
                    });
                }

                setIsExecuting(true);

                // Execute the retried step and all remaining steps using shared logic
                const result = await executeStepsWithTracking(
                    existingSteps,
                    stepIndex,
                    config,
                    mode,
                    chainPath,
                    dbRecordId,
                );

                if (result.success) {
                    // All steps completed successfully
                    if (dbRecordId != null) {
                        await UpdateTransactionStatus(db, {
                            id: dbRecordId,
                            status: "success",
                            currentStep: existingSteps.length,
                        });
                    }

                    transfer.completeTransfer();
                }

                return result;
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : "Unknown error";
                transfer.failTransfer(errorMessage);

                if (dbRecordId != null) {
                    await UpdateTransactionStatus(db, {
                        id: dbRecordId,
                        status: "failed",
                        error: errorMessage,
                    });
                }

                return { success: false, error: errorMessage };
            } finally {
                setIsExecuting(false);
            }
        },
        [db, transfer, executeStepsWithTracking],
    );

    const contextValue = useMemo(
        () => ({
            db,
            isDbReady,
            executeTransfer,
            executeStep,
            retryStep,
            isExecuting,
        }),
        [db, isDbReady, executeTransfer, executeStep, retryStep, isExecuting],
    );

    return <TaskContext.Provider value={contextValue}>{children}</TaskContext.Provider>;
};

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Get expected output amount from pathfinder response
 */
function getExpectedOutput(response: FindPathResponse): string {
    if (!response.success) return "0";

    switch (response.route.case) {
        case "direct":
            return response.route.value.transfer?.amount || "0";
        case "indirect": {
            const legs = response.route.value.legs;
            return legs[legs.length - 1]?.amount || "0";
        }
        case "brokerSwap": {
            const route = response.route.value;
            if (route.outboundLegs.length > 0) {
                return route.outboundLegs[route.outboundLegs.length - 1].amount;
            }
            return route.swap?.amountOut || "0";
        }
        default:
            return "0";
    }
}
