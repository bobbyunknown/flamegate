import { createFileRoute } from "@tanstack/react-router";
import { PlansPage } from "../../pages/Plans";

export const Route = createFileRoute("/_authed/plans")({
  component: PlansPage,
});
