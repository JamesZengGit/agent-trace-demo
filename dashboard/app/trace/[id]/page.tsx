import TraceDetailClient from '@/components/TraceDetailClient';

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <TraceDetailClient traceId={id} />;
}
