// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


public abstract class CrBase : IDisposable
{
    protected readonly World w = new();
    public void Dispose() => w.Dispose();
}
