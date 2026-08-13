import { createFileRoute } from "@tanstack/react-router";
import { UsagePage } from "../../pages/Usage";

export const Route = createFileRoute("/_authed/usage")({
  component: UsagePage,
});
