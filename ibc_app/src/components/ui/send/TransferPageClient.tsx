"use client";

import { useCallback, useMemo } from "react";

import type { ClientConfig } from "@/components/modules/tomlTypes";
import HistoryPanel from "@/components/ui/history/historyPanel";
import SendUI from "@/components/ui/send/senderUi";
import TransferTracker from "@/components/ui/tracking/transferTracker";
import { TaskProvider } from "@/context/taskProvider";
import { TransferProvider, useTransfer } from "@/context/transferContext";
import type { TransactionWithMeta } from "@/hooks/useTransactionHistory";

interface TransferPageClientProps {
    config: ClientConfig;
    initialSendChain?: string;
    initialReceiveChain?: string;
    initialSendToken?: string;
    initialReceiveToken?: string;
    initialAmount?: string;
}

/**
 * Inner component that uses the transfer context
 */
function TransferPageInner({
    config,
    initialSendChain = "",
    initialReceiveChain = "",
    initialSendToken = "",
    initialReceiveToken = "",
    initialAmount = "",
}: TransferPageClientProps) {
    const transfer = useTransfer();
    const { state, reset } = transfer;

    // Determine if we should show the tracking view
    const showTracker = useMemo(() => {
        return (
            state.phase === "preparing" ||
            state.phase === "executing" ||
            state.phase === "tracking" ||
            state.phase === "completed" ||
            state.phase === "failed"
        );
    }, [state.phase]);

    // Handle going back to transfer form
    const handleBackToTransfer = useCallback(() => {
        reset();
    }, [reset]);

    // Handle resuming a transaction from history
    const handleResume = useCallback((transaction: TransactionWithMeta) => {
        // TODO: Implement resume logic
        // This would need to:
        // 1. Load the transaction state from IndexedDB
        // 2. Reconstruct the steps array
        // 3. Set the transfer context state
        // 4. Resume execution from the current step
        console.log("Resume transaction:", transaction);
    }, []);

    // Handle retrying a failed transaction from history
    const handleRetry = useCallback((transaction: TransactionWithMeta) => {
        // TODO: Implement retry logic
        // Similar to resume but specifically for failed transactions
        console.log("Retry transaction:", transaction);
    }, []);

    // Show tracking view when transfer is active
    if (showTracker) {
        return <TransferTracker config={config} onBack={handleBackToTransfer} />;
    }

    // Show transfer form
    // On PC: Full viewport height with flex layout (no scroll)
    // On Mobile/Tablet: Normal scrollable layout
    return (
        <div className="w-full max-w-5xl mx-auto px-4 py-6 lg:py-0 lg:h-full lg:flex lg:flex-col lg:justify-center">
            <div className="space-y-4 lg:space-y-5">
                {/* History Panel - collapsible, above the form */}
                <HistoryPanel config={config} onResume={handleResume} onRetry={handleRetry} />

                {/* Transfer Form */}
                <SendUI
                    config={config}
                    sendChain={initialSendChain}
                    receiveChain={initialReceiveChain}
                    sendToken={initialSendToken}
                    receiveToken={initialReceiveToken}
                    amount={initialAmount}
                />
            </div>
        </div>
    );
}

/**
 * Client component for the transfer page
 * Wraps the inner component with necessary providers
 */
export default function TransferPageClient(props: TransferPageClientProps) {
    return (
        <TransferProvider>
            <TaskProvider>
                <TransferPageInner {...props} />
            </TaskProvider>
        </TransferProvider>
    );
}
