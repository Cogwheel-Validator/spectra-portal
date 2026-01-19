import { Particles } from "@/components/ui/particles";
import { LoadConfig } from "@/lib/config/config";
import TransferPageClient from "../../components/ui/send/TransferPageClient";

interface TransferPageProps {
    searchParams: Promise<{
        from_chain: string;
        to_chain: string;
        send_asset: string;
        receive_asset: string;
        amount: string;
    }>;
}

export default async function TransferPage(props: TransferPageProps) {
    const searchParams = await props.searchParams;
    const config = await LoadConfig("toml");

    if (!config) {
        return (
            <div className="min-h-screen flex items-center justify-center bg-slate-900">
                <div className="text-center">
                    <h1 className="text-2xl font-bold text-red-400">Configuration Error</h1>
                    <p className="text-slate-400 mt-2">Failed to load chain configuration.</p>
                </div>
            </div>
        );
    }

    // Fills available space in the flex layout
    // Particles expand to fill the entire area
    return (
        <div className="relative w-full h-full min-h-screen flex-1 bg-blend-soft-light bg-radial-[at_50%_65%] 
        from-slate-800 via-blue-950 to-indigo-950 to-90% pt-8 pb-40 lg:pt-16 lg:pb-32">
            <Particles className="absolute inset-0 z-0" />
            <div className="relative z-10 h-full">
                <TransferPageClient
                    config={config.config}
                    initialSendChain={searchParams.from_chain}
                    initialReceiveChain={searchParams.to_chain}
                    initialSendToken={searchParams.send_asset}
                    initialReceiveToken={searchParams.receive_asset}
                    initialAmount={searchParams.amount}
                />
            </div>
        </div>
    );
}
