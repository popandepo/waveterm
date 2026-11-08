// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;

namespace WaveNative.Protocol;

// The line message of pkg/wshutil/wshrpc.go (RpcMessage). Field names and omit-when-empty rules are the wire format the Go wsh reads and writes.
public sealed class RpcMessage
{
    public const string ControlRoute = "$control";

    [JsonPropertyName("command")] public string? Command { get; set; }
    [JsonPropertyName("reqid")] public string? ReqId { get; set; }
    [JsonPropertyName("resid")] public string? ResId { get; set; }
    [JsonPropertyName("timeout")] public long? Timeout { get; set; }
    [JsonPropertyName("route")] public string? Route { get; set; }
    [JsonPropertyName("source")] public string? Source { get; set; }
    [JsonPropertyName("cont")] public bool? Cont { get; set; }
    [JsonPropertyName("cancel")] public bool? Cancel { get; set; }
    [JsonPropertyName("error")] public string? Error { get; set; }
    [JsonPropertyName("datatype")] public string? DataType { get; set; }
    [JsonPropertyName("data")] public JsonNode? Data { get; set; }

    public bool IsRequest => !string.IsNullOrEmpty(Command) || !string.IsNullOrEmpty(ReqId);

    public static readonly JsonSerializerOptions Json = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    public static RpcMessage? Parse(string line) => JsonSerializer.Deserialize<RpcMessage>(line, Json);

    public string ToLine() => JsonSerializer.Serialize(this, Json);

    public static RpcMessage Response(string reqId, JsonNode? data) => new() { ResId = reqId, Data = data };

    public static RpcMessage ErrorResponse(string reqId, string error) => new() { ResId = reqId, Error = error };
}
