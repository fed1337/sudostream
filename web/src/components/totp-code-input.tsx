import { Controller, type Control, type FieldPath, type FieldValues } from "react-hook-form";

import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/input-otp";

type TotpCodeInputProps<TFieldValues extends FieldValues> = {
  control: Control<TFieldValues>;
  name: FieldPath<TFieldValues>;
  id?: string;
  disabled?: boolean;
  maxLength?: number;
  onComplete?: () => void;
};

export function TotpCodeInput<TFieldValues extends FieldValues>({
  control,
  name,
  id,
  disabled = false,
  maxLength = 6,
  onComplete,
}: TotpCodeInputProps<TFieldValues>) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <InputOTP
          id={id}
          maxLength={maxLength}
          value={field.value ?? ""}
          onChange={field.onChange}
          onBlur={field.onBlur}
          disabled={disabled}
          aria-invalid={fieldState.invalid}
          containerClassName="justify-center"
          onComplete={onComplete}
        >
          <InputOTPGroup>
            {Array.from({ length: maxLength }, (_, index) => (
              <InputOTPSlot key={index} index={index} />
            ))}
          </InputOTPGroup>
        </InputOTP>
      )}
    />
  );
}
