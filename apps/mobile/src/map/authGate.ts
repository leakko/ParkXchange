export type AuthGateOptions = {
  signedIn: boolean;
  confirmSignIn: () => Promise<boolean>;
  onRequireSignIn: () => void;
};

export async function passAuthGate({
  signedIn,
  confirmSignIn,
  onRequireSignIn,
}: AuthGateOptions): Promise<boolean> {
  if (signedIn) {
    return true;
  }
  if (await confirmSignIn()) {
    onRequireSignIn();
  }
  return false;
}
