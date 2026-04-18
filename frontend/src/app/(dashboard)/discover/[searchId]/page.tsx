"use client";

import { use, useEffect } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import { Card, CardContent } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getSearch } from "@/lib/api/discover";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";

type PageProps = { params: Promise<{ searchId: string }> };

const PHASE_LABEL: Record<string, string> = {
  processing: "Parsing Excel file...",
  enriching: "Verifying via Google Places and scraping websites...",
  scoring: "Classifying and scoring leads...",
  completed: "Done.",
  failed: "Failed.",
};

export default function DiscoveryProgressPage({ params }: PageProps) {
  const { searchId } = use(params);
  const router = useRouter();

  const { data: search, error } = useSWR(
    searchId ? `/discover/${searchId}` : null,
    () => getSearch(searchId),
    {
      refreshInterval: (latest) =>
        latest?.status === "completed" || latest?.status === "failed" ? 0 : 3000,
    },
  );

  useEffect(() => {
    if (search?.status === "completed") {
      const timer = setTimeout(() => {
        router.replace("/markets");
      }, 1200);
      return () => clearTimeout(timer);
    }
  }, [search?.status, router]);

  if (error) {
    return (
      <div className="max-w-xl mx-auto py-20">
        <Alert variant="destructive">
          <AlertDescription>Failed to load discovery status</AlertDescription>
        </Alert>
      </div>
    );
  }

  const status = search?.status ?? "processing";
  const phase = PHASE_LABEL[status] ?? status;
  const marketCount = search?.query?.market_ids?.length ?? 0;

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh]">
      <Card className="w-full max-w-lg">
        <CardContent className="pt-10 pb-10 text-center">
          {status === "completed" ? (
            <>
              <CheckCircle2 className="h-10 w-10 mx-auto text-green-600 mb-4" />
              <h2 className="text-xl font-semibold mb-1">Done</h2>
              <p className="text-sm text-muted-foreground">Redirecting to your markets…</p>
            </>
          ) : status === "failed" ? (
            <>
              <XCircle className="h-10 w-10 mx-auto text-red-600 mb-4" />
              <h2 className="text-xl font-semibold mb-1">Upload Failed</h2>
              <p className="text-sm text-muted-foreground">
                Something went wrong processing your Excel file.
              </p>
            </>
          ) : (
            <>
              <Loader2 className="h-10 w-10 mx-auto animate-spin text-blue-600 mb-4" />
              <h2 className="text-xl font-semibold mb-1">Processing your shipment data</h2>
              <p className="text-sm text-muted-foreground">{phase}</p>
              {marketCount > 0 && (
                <p className="text-xs text-muted-foreground mt-4">
                  {marketCount} market{marketCount !== 1 ? "s" : ""} being processed
                </p>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
