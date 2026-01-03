import { Particles } from "@/components/ui/particles";
import SendUI from "@/components/ui/send/senderUi";
import { LoadConfig } from "@/lib/config/config";

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
    // TODO: maybe as some other way to specify what should app load (toml or json)
    const config = await LoadConfig("toml");
    if (!config) {
        return <div>Error loading config</div>;
    }

    return (
        <div className="relative w-full min-h-screen bg-blend-soft-light bg-radial-[at_50%_65%] from-slate-800 via-blue-950 to-indigo-950 to-90%">
            <Particles className="absolute inset-0 z-0" />
            <SendUI
                config={config.config}
                sendChain={searchParams.from_chain}
                receiveChain={searchParams.to_chain}
                sendToken={searchParams.send_asset}
                receiveToken={searchParams.receive_asset}
                amount={searchParams.amount}
            />
        </div>
    );
}
