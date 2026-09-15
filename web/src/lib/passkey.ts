import { json } from "./api";
const decode = (v: string) =>
  Uint8Array.from(atob(v.replace(/-/g, "+").replace(/_/g, "/")), (c) =>
    c.charCodeAt(0),
  );
const encode = (v: ArrayBuffer) =>
  btoa(String.fromCharCode(...new Uint8Array(v)))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
export async function passkey(register: boolean) {
  const action = register ? "register" : "login";
  const options = await json<any>("/passkey/" + action + "/begin", {
    method: "POST",
  });
  const key = options.publicKey;
  key.challenge = decode(key.challenge);
  if (key.user) key.user.id = decode(key.user.id);
  for (const field of ["excludeCredentials", "allowCredentials"])
    if (key[field])
      key[field] = key[field].map((c: any) => ({ ...c, id: decode(c.id) }));
  const credential = (await (register
    ? navigator.credentials.create(options)
    : navigator.credentials.get(options))) as PublicKeyCredential | null;
  if (!credential) throw Error("No passkey was selected.");
  const response = credential.response as AuthenticatorAttestationResponse &
    AuthenticatorAssertionResponse;
  const result = await json<{ success?: boolean; redirect?: string }>(
    "/passkey/" + action + "/finish",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: credential.id,
        rawId: encode(credential.rawId),
        type: credential.type,
        authenticatorAttachment: credential.authenticatorAttachment,
        clientExtensionResults: credential.getClientExtensionResults(),
        response: {
          clientDataJSON: encode(response.clientDataJSON),
          ...(register
            ? {
                attestationObject: encode(response.attestationObject),
                transports: response.getTransports?.(),
              }
            : {
                authenticatorData: encode(response.authenticatorData),
                signature: encode(response.signature),
                userHandle: response.userHandle
                  ? encode(response.userHandle)
                  : null,
              }),
        },
      }),
    },
  );
  if (result.success === false) throw Error("Passkey verification failed.");
  return result;
}
