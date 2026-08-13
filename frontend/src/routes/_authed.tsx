import { createFileRoute } from "@tanstack/react-router";
import { AuthGate } from "../components/AuthGate";
import { Layout } from "../components/Layout";
import { AdminBrandingProvider } from "../contexts/BrandingContext";

export const Route = createFileRoute("/_authed")({
  component: AuthedLayout,
});

function AuthedLayout() {
  return (
    <AuthGate>
      <AdminBrandingProvider>
        <Layout />
      </AdminBrandingProvider>
    </AuthGate>
  );
}
