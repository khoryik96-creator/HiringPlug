const encoder = new TextEncoder(),
  decoder = new TextDecoder();
const HEX = /^[a-f0-9]{64}$/;

export const random = () =>
  Array.from(crypto.getRandomValues(new Uint8Array(32)), (b) => b.toString(16).padStart(2, "0")).join("");
export const digest = async (text) =>
  Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(text))), (b) =>
    b.toString(16).padStart(2, "0"),
  ).join("");
const b64 = (bytes) => btoa(String.fromCharCode(...bytes));
const bytes = (value) => Uint8Array.from(atob(value), (c) => c.charCodeAt(0));
async function key(env) {
  if (!HEX.test(env.TOKEN_KEY || "")) throw Error("configuration");
  return crypto.subtle.importKey(
    "raw",
    Uint8Array.from(env.TOKEN_KEY.match(/../g), (h) => parseInt(h, 16)),
    "AES-GCM",
    false,
    ["encrypt", "decrypt"],
  );
}
export async function seal(env, value, identity) {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encrypted = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv, additionalData: encoder.encode(identity) },
    await key(env),
    encoder.encode(JSON.stringify(value)),
  );
  return b64(iv) + "." + b64(new Uint8Array(encrypted));
}
export async function unseal(env, value, identity) {
  const [iv, data] = value.split(".");
  return JSON.parse(
    decoder.decode(
      await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: bytes(iv), additionalData: encoder.encode(identity) },
        await key(env),
        bytes(data),
      ),
    ),
  );
}
