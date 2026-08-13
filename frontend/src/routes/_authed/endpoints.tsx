import { createFileRoute } from "@tanstack/react-router";
import { EndpointsPage } from "../../pages/Endpoints";

export const Route = createFileRoute("/_authed/endpoints")({
  component: EndpointsPage,
});
