import { createFileRoute } from "@tanstack/react-router";
import { QuotaPage } from "../../pages/Quota";

export const Route = createFileRoute("/_authed/quota")({
  component: QuotaPage,
});
