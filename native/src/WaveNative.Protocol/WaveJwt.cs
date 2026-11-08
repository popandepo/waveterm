// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using Org.BouncyCastle.Crypto.Parameters;
using Org.BouncyCastle.Crypto.Signers;

namespace WaveNative.Protocol;

// The claims of pkg/wavejwt/wavejwt.go. The Go wsh reads sock, routeid and blockid out of the token without verifying it; the server verifies the EdDSA signature.
public sealed class WaveClaims
{
    public string? Sock { get; set; }
    public string? RouteId { get; set; }
    public bool ProcRoute { get; set; }
    public string? BlockId { get; set; }
    public string? Conn { get; set; }
    public bool Router { get; set; }

    public string GenerateRouteId() => !string.IsNullOrEmpty(RouteId) ? RouteId : "";
}

public sealed class WaveJwt
{
    const string Issuer = "waveterm";
    readonly Ed25519PrivateKeyParameters priv;
    readonly Ed25519PublicKeyParameters pub;

    public WaveJwt(Ed25519PrivateKeyParameters privateKey)
    {
        priv = privateKey;
        pub = privateKey.GeneratePublicKey();
    }

    public static WaveJwt Generate() => new(new Ed25519PrivateKeyParameters(new Org.BouncyCastle.Security.SecureRandom()));

    public string Sign(WaveClaims c)
    {
        var now = DateTimeOffset.UtcNow;
        var claims = new JsonObject { ["iss"] = Issuer, ["iat"] = now.ToUnixTimeSeconds(), ["exp"] = now.AddDays(365).ToUnixTimeSeconds() };
        if (!string.IsNullOrEmpty(c.Sock)) claims["sock"] = c.Sock;
        if (!string.IsNullOrEmpty(c.RouteId)) claims["routeid"] = c.RouteId;
        if (c.ProcRoute) claims["procroute"] = true;
        if (!string.IsNullOrEmpty(c.BlockId)) claims["blockid"] = c.BlockId;
        if (!string.IsNullOrEmpty(c.Conn)) claims["conn"] = c.Conn;
        if (c.Router) claims["router"] = true;
        var head = B64(Encoding.UTF8.GetBytes("{\"alg\":\"EdDSA\",\"typ\":\"JWT\"}"));
        var body = B64(Encoding.UTF8.GetBytes(claims.ToJsonString()));
        var signingInput = Encoding.ASCII.GetBytes(head + "." + body);
        var signer = new Ed25519Signer();
        signer.Init(true, priv);
        signer.BlockUpdate(signingInput, 0, signingInput.Length);
        return head + "." + body + "." + B64(signer.GenerateSignature());
    }

    // Returns null when the token is malformed, uses another algorithm, has a bad signature, or is expired.
    public WaveClaims? Validate(string token)
    {
        var parts = token.Split('.');
        if (parts.Length != 3) return null;
        try
        {
            var header = JsonNode.Parse(Encoding.UTF8.GetString(Unb64(parts[0])));
            if (header?["alg"]?.GetValue<string>() != "EdDSA") return null;
            var signingInput = Encoding.ASCII.GetBytes(parts[0] + "." + parts[1]);
            var verifier = new Ed25519Signer();
            verifier.Init(false, pub);
            verifier.BlockUpdate(signingInput, 0, signingInput.Length);
            if (!verifier.VerifySignature(Unb64(parts[2]))) return null;
            var claims = JsonNode.Parse(Encoding.UTF8.GetString(Unb64(parts[1])))!;
            if (claims["iss"]?.GetValue<string>() != Issuer) return null;
            var exp = claims["exp"]?.GetValue<long>();
            if (exp != null && DateTimeOffset.UtcNow.ToUnixTimeSeconds() > exp) return null;
            return new WaveClaims
            {
                Sock = claims["sock"]?.GetValue<string>(),
                RouteId = claims["routeid"]?.GetValue<string>(),
                ProcRoute = claims["procroute"]?.GetValue<bool>() ?? false,
                BlockId = claims["blockid"]?.GetValue<string>(),
                Conn = claims["conn"]?.GetValue<string>(),
                Router = claims["router"]?.GetValue<bool>() ?? false,
            };
        }
        catch (Exception e) when (e is FormatException or JsonException or InvalidOperationException)
        {
            return null;
        }
    }

    static string B64(byte[] b) => Convert.ToBase64String(b).TrimEnd('=').Replace('+', '-').Replace('/', '_');

    static byte[] Unb64(string s)
    {
        s = s.Replace('-', '+').Replace('_', '/');
        return Convert.FromBase64String(s.PadRight(s.Length + (4 - s.Length % 4) % 4, '='));
    }
}
