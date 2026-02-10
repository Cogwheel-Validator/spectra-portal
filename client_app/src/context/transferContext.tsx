"use client";

import { createContext, type ReactNode, useCallback, useContext, useReducer } from "react";
import type { ClientToken } from "@/components/modules/tomlTypes";
import type { FindPathResponse, WasmData } from "@/lib/generated/pathfinder/pathfinder_route_pb";

// ============================================================================
// Types
// ============================================================================

export type TransferMode = "manual" | "smart";
export type TransferPhase =
    | "idle"
    | "preparing"
    | "executing"
    | "tracking"
    | "completed"
    | "failed";
export type RouteType = "direct" | "indirect" | "brokerSwap";

/**
 * Represents a single step in the transfer process
 */
export interface TransferStep {
    id: string;
    type: "ibc_transfer" | "swap" | "multi-hop" | "wasm-execution" | "multi-hop + swap";
    fromChain: string;
    toChain: string;
    status: "pending" | "signing" | "broadcasting" | "confirming" | "completed" | "failed";
    txHash?: string;
    error?: string;
    metadata?: {
        channel?: string;
        port?: string;
        amount?: string;
        denom?: string;
        memo?: string;
        smartContractData?: WasmData;
    };
    // Only applicable for multi-hop, multi-hop + swap and wasm-execution if the transfer mode is "smart",
    // should store path data, or in this case chain ids. Usable for tracking. If the transfer mode is "manual",
    // this is not applicable.
    transferOrder?: string[];
}

/**
 * Core transfer state
 */
export interface TransferState {
    phase: TransferPhase;
    mode: TransferMode;
    routeType: RouteType | null;

    // Chain and token selections
    fromChainId: string;
    toChainId: string;
    fromToken: ClientToken | null;
    toToken: ClientToken | null;
    amount: string;

    // Addresses
    senderAddress: string;
    receiverAddress: string;

    // Slippage tolerance in basis points (100 = 1%)
    slippageBps: number;

    // Pathfinder response
    pathfinderResponse: FindPathResponse | null;

    // Execution state
    steps: TransferStep[];
    currentStepIndex: number;
    dbRecordId: number | null;

    // Error state
    error: string | null;

    // Chain path for visualization
    chainPath: string[];

    // Multi-hop tracking: 0–100 while tracking (null when not in use)
    multiHopProgress: number | null;
    // Total chains in path for multi-hop (e.g. 4 for 25% per chain)
    multiHopTotalChains: number | null;
}

// ============================================================================
// Actions
// ============================================================================

type TransferAction =
    | { type: "SET_MODE"; payload: TransferMode }
    | { type: "SET_FROM_CHAIN"; payload: string }
    | { type: "SET_TO_CHAIN"; payload: string }
    | { type: "SET_FROM_TOKEN"; payload: ClientToken | null }
    | { type: "SET_TO_TOKEN"; payload: ClientToken | null }
    | { type: "SET_AMOUNT"; payload: string }
    | { type: "SET_SENDER_ADDRESS"; payload: string }
    | { type: "SET_RECEIVER_ADDRESS"; payload: string }
    | { type: "SET_SLIPPAGE"; payload: number }
    | { type: "SET_PATHFINDER_RESPONSE"; payload: FindPathResponse | null }
    | { type: "START_PREPARING" }
    | {
          type: "START_EXECUTING";
          payload: { steps: TransferStep[]; dbRecordId: number; chainPath: string[] };
      }
    | { type: "UPDATE_STEP"; payload: { index: number; step: Partial<TransferStep> } }
    | { type: "ADVANCE_STEP" }
    | { type: "COMPLETE_TRANSFER" }
    | { type: "FAIL_TRANSFER"; payload: string }
    | { type: "RESET" }
    | {
          type: "RESUME_TRANSFER";
          payload: {
              steps: TransferStep[];
              currentStepIndex: number;
              dbRecordId: number;
              chainPath: string[];
          };
      }
    | {
          type: "SET_MULTIHOP_PROGRESS";
          payload: { progress: number | null; totalChains?: number | null };
      };

// ============================================================================
// Initial State
// ============================================================================

const initialState: TransferState = {
    phase: "idle",
    mode: "smart",
    routeType: null,
    fromChainId: "",
    toChainId: "",
    fromToken: null,
    toToken: null,
    amount: "",
    senderAddress: "",
    receiverAddress: "",
    slippageBps: 100, // Default 1% slippage
    pathfinderResponse: null,
    steps: [],
    currentStepIndex: 0,
    dbRecordId: null,
    error: null,
    chainPath: [],
    multiHopProgress: null,
    multiHopTotalChains: null,
};

// ============================================================================
// Reducer
// ============================================================================

/**
 * A reducer function to update the state of the transfer.
 * @param state - The current state of the transfer
 * @param action - The action to perform
 * @returns The new state of the transfer
 *
 * This function is used to update the state of the transfer.
 * It is a reducer function that takes the current state and an action and returns the new state.
 * The action is an object that contains the type of action to perform and the payload of the action.
 * The payload is the data that is needed to perform the action.
 */
function transferReducer(state: TransferState, action: TransferAction): TransferState {
    switch (action.type) {
        case "SET_MODE":
            return { ...state, mode: action.payload };

        case "SET_FROM_CHAIN":
            return {
                ...state,
                fromChainId: action.payload,
                fromToken: null,
                toChainId: "",
                toToken: null,
                amount: "",
                pathfinderResponse: null,
            };

        case "SET_TO_CHAIN":
            return {
                ...state,
                toChainId: action.payload,
                toToken: null,
                pathfinderResponse: null,
            };

        case "SET_FROM_TOKEN":
            return {
                ...state,
                fromToken: action.payload,
                pathfinderResponse: null,
            };

        case "SET_TO_TOKEN":
            return {
                ...state,
                toToken: action.payload,
                pathfinderResponse: null,
            };

        case "SET_AMOUNT":
            return { ...state, amount: action.payload };

        case "SET_SENDER_ADDRESS":
            return { ...state, senderAddress: action.payload };

        case "SET_RECEIVER_ADDRESS":
            return { ...state, receiverAddress: action.payload };

        case "SET_SLIPPAGE":
            return { ...state, slippageBps: action.payload };

        case "SET_PATHFINDER_RESPONSE": {
            const response = action.payload;
            let routeType: RouteType | null = null;
            if (response?.success && response.route.case) {
                routeType = response.route.case as RouteType;
            }
            return {
                ...state,
                pathfinderResponse: response,
                routeType,
            };
        }

        case "START_PREPARING":
            return { ...state, phase: "preparing", error: null };

        case "START_EXECUTING":
            return {
                ...state,
                phase: "executing",
                steps: action.payload.steps,
                currentStepIndex: 0,
                dbRecordId: action.payload.dbRecordId,
                chainPath: action.payload.chainPath,
                error: null,
            };

        case "UPDATE_STEP": {
            const newSteps = [...state.steps];
            newSteps[action.payload.index] = {
                ...newSteps[action.payload.index],
                ...action.payload.step,
            };
            return { ...state, steps: newSteps };
        }

        case "ADVANCE_STEP": {
            const nextIndex = state.currentStepIndex + 1;
            const newPhase = nextIndex >= state.steps.length ? "tracking" : state.phase;
            return {
                ...state,
                currentStepIndex: nextIndex,
                phase: newPhase,
            };
        }

        case "COMPLETE_TRANSFER":
            return {
                ...state,
                phase: "completed",
                error: null,
            };

        case "FAIL_TRANSFER":
            return {
                ...state,
                phase: "failed",
                error: action.payload,
            };

        case "RESET":
            return { ...initialState, mode: state.mode };

        case "RESUME_TRANSFER":
            return {
                ...state,
                phase: "executing",
                steps: action.payload.steps,
                currentStepIndex: action.payload.currentStepIndex,
                dbRecordId: action.payload.dbRecordId,
                chainPath: action.payload.chainPath,
                error: null,
            };

        case "SET_MULTIHOP_PROGRESS":
            return {
                ...state,
                multiHopProgress: action.payload.progress,
                multiHopTotalChains:
                    action.payload.totalChains !== undefined
                        ? action.payload.totalChains
                        : state.multiHopTotalChains,
            };

        default:
            return state;
    }
}

// ============================================================================
// Context
// ============================================================================

interface TransferContextType {
    state: TransferState;

    // Selection actions
    setMode: (mode: TransferMode) => void;
    setFromChain: (chainId: string) => void;
    setToChain: (chainId: string) => void;
    setFromToken: (token: ClientToken | null) => void;
    setToToken: (token: ClientToken | null) => void;
    setAmount: (amount: string) => void;
    setSenderAddress: (address: string) => void;
    setReceiverAddress: (address: string) => void;
    setSlippage: (slippageBps: number) => void;
    setPathfinderResponse: (response: FindPathResponse | null) => void;

    // Execution actions
    startPreparing: () => void;
    startExecuting: (steps: TransferStep[], dbRecordId: number, chainPath: string[]) => void;
    updateStep: (index: number, step: Partial<TransferStep>) => void;
    advanceStep: () => void;
    completeTransfer: () => void;
    failTransfer: (error: string) => void;
    reset: () => void;
    resumeTransfer: (
        steps: TransferStep[],
        currentStepIndex: number,
        dbRecordId: number,
        chainPath: string[],
    ) => void;
    setMultiHopProgress: (progress: number | null, totalChains?: number | null) => void;

    // Computed properties
    canInitiateTransfer: boolean;
    isTransferActive: boolean;
    currentStep: TransferStep | null;
    progress: number;
}

const TransferContext = createContext<TransferContextType | undefined>(undefined);

// ============================================================================
// Provider
// ============================================================================

export function TransferProvider({ children }: { children: ReactNode }) {
    const [state, dispatch] = useReducer(transferReducer, initialState);

    // Selection actions
    const setMode = useCallback((mode: TransferMode) => {
        dispatch({ type: "SET_MODE", payload: mode });
    }, []);

    const setFromChain = useCallback((chainId: string) => {
        dispatch({ type: "SET_FROM_CHAIN", payload: chainId });
    }, []);

    const setToChain = useCallback((chainId: string) => {
        dispatch({ type: "SET_TO_CHAIN", payload: chainId });
    }, []);

    const setFromToken = useCallback((token: ClientToken | null) => {
        dispatch({ type: "SET_FROM_TOKEN", payload: token });
    }, []);

    const setToToken = useCallback((token: ClientToken | null) => {
        dispatch({ type: "SET_TO_TOKEN", payload: token });
    }, []);

    const setAmount = useCallback((amount: string) => {
        dispatch({ type: "SET_AMOUNT", payload: amount });
    }, []);

    const setSenderAddress = useCallback((address: string) => {
        dispatch({ type: "SET_SENDER_ADDRESS", payload: address });
    }, []);

    const setReceiverAddress = useCallback((address: string) => {
        dispatch({ type: "SET_RECEIVER_ADDRESS", payload: address });
    }, []);

    const setSlippage = useCallback((slippageBps: number) => {
        dispatch({ type: "SET_SLIPPAGE", payload: slippageBps });
    }, []);

    const setPathfinderResponse = useCallback((response: FindPathResponse | null) => {
        dispatch({ type: "SET_PATHFINDER_RESPONSE", payload: response });
    }, []);

    // Execution actions
    const startPreparing = useCallback(() => {
        dispatch({ type: "START_PREPARING" });
    }, []);

    const startExecuting = useCallback(
        (steps: TransferStep[], dbRecordId: number, chainPath: string[]) => {
            dispatch({ type: "START_EXECUTING", payload: { steps, dbRecordId, chainPath } });
        },
        [],
    );

    const updateStep = useCallback((index: number, step: Partial<TransferStep>) => {
        dispatch({ type: "UPDATE_STEP", payload: { index, step } });
    }, []);

    const advanceStep = useCallback(() => {
        dispatch({ type: "ADVANCE_STEP" });
    }, []);

    const completeTransfer = useCallback(() => {
        dispatch({ type: "COMPLETE_TRANSFER" });
    }, []);

    const failTransfer = useCallback((error: string) => {
        dispatch({ type: "FAIL_TRANSFER", payload: error });
    }, []);

    const reset = useCallback(() => {
        dispatch({ type: "RESET" });
    }, []);

    const resumeTransfer = useCallback(
        (
            steps: TransferStep[],
            currentStepIndex: number,
            dbRecordId: number,
            chainPath: string[],
        ) => {
            dispatch({
                type: "RESUME_TRANSFER",
                payload: { steps, currentStepIndex, dbRecordId, chainPath },
            });
        },
        [],
    );

    const setMultiHopProgress = useCallback(
        (progress: number | null, totalChains?: number | null) => {
            dispatch({
                type: "SET_MULTIHOP_PROGRESS",
                payload: { progress, totalChains },
            });
        },
        [],
    );

    // Computed properties
    const canInitiateTransfer =
        state.phase === "idle" &&
        state.fromChainId !== "" &&
        state.toChainId !== "" &&
        state.fromToken !== null &&
        state.amount !== "" &&
        Number.parseFloat(state.amount) > 0 &&
        state.senderAddress !== "" &&
        state.receiverAddress !== "" &&
        state.pathfinderResponse?.success === true;

    const isTransferActive =
        state.phase === "preparing" || state.phase === "executing" || state.phase === "tracking";

    const currentStep = state.steps[state.currentStepIndex] ?? null;

    const progress =
        state.steps.length > 0
            ? Math.round((state.currentStepIndex / state.steps.length) * 100)
            : 0;

    return (
        <TransferContext.Provider
            value={{
                state,
                setMode,
                setFromChain,
                setToChain,
                setFromToken,
                setToToken,
                setAmount,
                setSenderAddress,
                setReceiverAddress,
                setSlippage,
                setPathfinderResponse,
                startPreparing,
                startExecuting,
                updateStep,
                advanceStep,
                completeTransfer,
                failTransfer,
                reset,
                resumeTransfer,
                setMultiHopProgress,
                canInitiateTransfer,
                isTransferActive,
                currentStep,
                progress,
            }}
        >
            {children}
        </TransferContext.Provider>
    );
}

// ============================================================================
// Hook
// ============================================================================

export function useTransfer() {
    const context = useContext(TransferContext);
    if (!context) {
        throw new Error("useTransfer must be used within TransferProvider");
    }
    return context;
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Generates transfer steps from a pathfinder response
 */
export function generateStepsFromResponse(
    response: FindPathResponse,
    mode: TransferMode,
): TransferStep[] {
    if (!response.success) return [];

    const steps: TransferStep[] = [];

    switch (response.route.case) {
        case "direct": {
            const transfer = response.route.value.transfer;
            if (transfer) {
                steps.push({
                    id: "direct-transfer",
                    type: "ibc_transfer",
                    fromChain: transfer.fromChain,
                    toChain: transfer.toChain,
                    status: "pending",
                    metadata: {
                        channel: transfer.channel,
                        port: transfer.port,
                        amount: transfer.amount,
                        denom: transfer.token?.chainDenom,
                    },
                });
            }
            break;
        }

        case "indirect": {
            const route = response.route.value;
            if (mode === "smart" && route.supportsPfm) {
                // Single PFM transaction
                steps.push({
                    id: "multi-hop",
                    type: "multi-hop",
                    fromChain: route.pfmStartChain,
                    // Because that is where the assets need to be sent in order to be forwarded
                    toChain: route.path[1],
                    status: "pending",
                    metadata: {
                        channel: route.legs[0].channel,
                        port: route.legs[0].port,
                        amount: route.legs[0].amount,
                        denom: route.legs[0].token?.chainDenom,
                        memo: route.pfmMemo,
                    },
                    transferOrder: route.path,
                });
            } else {
                // Manual: each leg is a separate step
                route.legs.forEach((leg, index) => {
                    steps.push({
                        id: `leg-${index}`,
                        type: "ibc_transfer",
                        fromChain: leg.fromChain,
                        toChain: leg.toChain,
                        status: "pending",
                        metadata: {
                            channel: leg.channel,
                            port: leg.port,
                            amount: leg.amount,
                            denom: leg.token?.chainDenom,
                        },
                    });
                });
            }
            break;
        }

        case "brokerSwap": {
            const route = response.route.value;
            const execution = route.execution ?? undefined;

            if (
                mode === "smart" &&
                execution?.usesWasm &&
                execution?.smartContractData !== undefined &&
                route.inboundLegs.length === 0
            ) {
                // Single WASM execution handles everything, and the start chain is the broker chain
                steps.push({
                    id: "wasm-execution",
                    type: "wasm-execution",
                    fromChain: route.path[0],
                    // Only maybe needed for some visual aspect
                    toChain: route.path[route.path.length - 1],
                    status: "pending",
                    metadata: {
                        smartContractData: execution.smartContractData,
                    },
                    transferOrder: route.path,
                });
            } else if (mode === "smart" && execution?.memo && execution.memo.length > 0) {
                // Multi-hop + swap because we have a memo with a PFM forward and a swap
                steps.push({
                    id: "multi-hop-swap",
                    type: "multi-hop + swap",
                    fromChain: route.path[0],
                    // Because that is where the assets need to be sent in order to be forwarded
                    // and the wasm-swap is on the second chain or forwarded to
                    toChain: route.path[1],
                    status: "pending",
                    metadata: {
                        channel: route.inboundLegs[0].channel,
                        port: route.inboundLegs[0].port,
                        amount: route.inboundLegs[0].amount,
                        denom: route.inboundLegs[0].token?.chainDenom,
                        memo: execution.memo,
                    },
                    transferOrder: route.path,
                });
            } else {
                // Manual: separate steps for inbound, swap, outbound
                if (route.inboundLegs.length > 0) {
                    route.inboundLegs.forEach((leg, i) => {
                        steps.push({
                            id: `inbound-${i + 1}`,
                            type: "ibc_transfer",
                            fromChain: leg.fromChain,
                            toChain: leg.toChain,
                            status: "pending",
                            metadata: {
                                channel: leg.channel,
                                port: leg.port,
                                amount: leg.amount,
                                denom: leg.token?.chainDenom,
                            },
                        });
                    });
                }

                if (route.swap) {
                    steps.push({
                        id: "swap",
                        type: "swap",
                        fromChain: route.path[route.inboundLegs.length],
                        // it is a swap hence why just same to chain
                        toChain: route.path[route.inboundLegs.length],
                        status: "pending",
                        metadata: {
                            amount: route.swap.amountIn,
                            denom: route.swap.tokenIn?.chainDenom,
                        },
                    });
                }

                route.outboundLegs.forEach((leg, index) => {
                    steps.push({
                        id: `outbound-${index}`,
                        type: "ibc_transfer",
                        fromChain: leg.fromChain,
                        toChain: leg.toChain,
                        status: "pending",
                        metadata: {
                            channel: leg.channel,
                            port: leg.port,
                            amount: leg.amount,
                            denom: leg.token?.chainDenom,
                        },
                    });
                });
            }
            break;
        }
    }

    return steps;
}

/**
 * Extracts the chain path from a pathfinder response
 */
export function extractChainPath(response: FindPathResponse): string[] {
    if (!response.success) return [];

    switch (response.route.case) {
        case "direct": {
            const transfer = response.route.value.transfer;
            return transfer ? [transfer.fromChain, transfer.toChain] : [];
        }
        case "indirect":
            return response.route.value.path;
        case "brokerSwap":
            return response.route.value.path;
        default:
            return [];
    }
}
