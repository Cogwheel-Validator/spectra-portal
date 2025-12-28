import SendUI from '@/components/ui/send/senderUi';
import { LoadConfig } from '@/lib/config';

interface TransferPageProps {
  searchParams: {
    from_chain: string;
    to_chain: string;
    send_asset: string;
    receive_asset: string;
    amount: string;
  };
}

export default async function TransferPage({ searchParams }: TransferPageProps) {
  // Load config at build/request time (cached)
  const config = await LoadConfig('toml');
  if (!config) {
    return <div>Error loading config</div>;
  }

  return (
    <SendUI
      config={config.config}
      sendChain={searchParams.from_chain}
      receiveChain={searchParams.to_chain}
      sendToken={searchParams.send_asset}
      receiveToken={searchParams.receive_asset}
      amount={searchParams.amount}
    />
  );
}