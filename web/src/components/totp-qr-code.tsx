import { QRCodeSVG } from "qrcode.react";

type TotpQrCodeProps = {
  otpauthURL: string;
  label: string;
};

export function TotpQrCode({ otpauthURL, label }: TotpQrCodeProps) {
  return (
    <div className="flex flex-col items-center gap-2">
      <QRCodeSVG
        value={otpauthURL}
        size={176}
        level="M"
        className="rounded-lg border bg-background p-2"
        aria-label={label}
      />
    </div>
  );
}
