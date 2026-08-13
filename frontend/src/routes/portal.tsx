import { createFileRoute } from "@tanstack/react-router";
import { PortalBrandingProvider } from "../contexts/BrandingContext";
import { KeyPortalPage } from "../pages/KeyPortal";

export const Route = createFileRoute("/portal")({
  component: () => (
    <PortalBrandingProvider>
      <KeyPortalPage />
    </PortalBrandingProvider>
  ),
});
