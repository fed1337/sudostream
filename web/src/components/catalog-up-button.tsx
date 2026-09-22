import { Link } from "react-router";
import { ArrowLeftIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

type CatalogUpButtonProps = {
  to: string;
  /** Visible label (e.g. Back). */
  label: string;
};

/** Parent navigation control matching admin back links. */
export function CatalogUpButton({ to, label }: CatalogUpButtonProps) {
  return (
    <Button variant="ghost" className="w-fit px-0" asChild>
      <Link to={to}>
        <ArrowLeftIcon data-icon="inline-start" />
        {label}
      </Link>
    </Button>
  );
}
