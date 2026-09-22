import { SunMoon, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  const cycleTheme = () => {
    switch (theme) {
      case "light":
        setTheme("dark");
        break;
      case "dark":
        setTheme("system");
        break;
      default:
        setTheme("light");
    }
  };

  const Icon = theme === "light" ? Sun : theme === "dark" ? Moon : SunMoon;

  return (
    <Button variant="ghost" size="icon" onClick={cycleTheme} aria-label={`Current theme: ${theme}`}>
      <Icon />
    </Button>
  );
}
