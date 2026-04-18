import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center">
        <h1 className="text-6xl font-bold text-blue-800 font-mono">404</h1>
        <p className="mt-2 text-muted-foreground">Page not found</p>
        <Link
          href="/"
          className="mt-6 inline-flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/80 cursor-pointer"
        >
          Back to Dashboard
        </Link>
      </div>
    </div>
  );
}
