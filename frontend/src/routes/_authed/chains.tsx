import { createFileRoute } from "@tanstack/react-router";
import { ChainsPage } from "../../pages/Chains";

export const Route = createFileRoute("/_authed/chains")({
  component: ChainsPage,
});
