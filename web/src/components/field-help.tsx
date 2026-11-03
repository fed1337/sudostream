import type { ReactNode } from "react";
import { CircleHelpIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

type FieldHelpProps = {
  label: string;
  children: ReactNode;
};

/** Popover help next to a field label (CircleHelp trigger). */
export function FieldHelp({ label, children }: FieldHelpProps) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-6 text-muted-foreground hover:text-foreground"
          aria-label={label}
        >
          <CircleHelpIcon />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="max-w-sm whitespace-pre-line text-sm">
        {children}
      </PopoverContent>
    </Popover>
  );
}
