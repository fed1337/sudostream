import { LoginForm } from "@/components/login-form";
import { GuestShell } from "@/components/guest-shell";

export default function LoginPage() {
  return (
    <GuestShell>
      <div className="flex w-full max-w-md flex-col gap-6 md:gap-8">
        <LoginForm />
      </div>
    </GuestShell>
  );
}
