// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Serialization;

namespace WaveNative.Layout;

// Port of frontend/layout/lib/types.ts (the parts the tree code uses). The JSON names are the ones the saved layouts carry.

public enum FlexDirection
{
    Row,
    Column,
}

public enum DropDirection
{
    Top = 0,
    Right = 1,
    Bottom = 2,
    Left = 3,
    OuterTop = 4,
    OuterRight = 5,
    OuterBottom = 6,
    OuterLeft = 7,
    Center = 8,
}

public enum LayoutTreeActionType
{
    ComputeMove,
    Move,
    Swap,
    Stack,
    Unstack,
    StackInsert,
    StackMove,
    StackReorder,
    TurnIntoStack,
    TurnStackOff,
    Activate,
    ResizeNode,
    InsertNode,
    InsertNodeAtIndex,
    DeleteNode,
    FocusNode,
    MagnifyNodeToggle,
    ClearTree,
    ReplaceNode,
    SplitHorizontal,
    SplitVertical,
}

public sealed class TabLayoutData
{
    [JsonPropertyName("blockId")] public string? BlockId { get; set; }
}

public sealed class LayoutNode
{
    public const double DefaultNodeSize = 10;

    [JsonPropertyName("id")] public string Id { get; set; } = "";
    [JsonPropertyName("data")] public TabLayoutData? Data { get; set; }
    [JsonPropertyName("children")] public List<LayoutNode>? Children { get; set; }
    [JsonPropertyName("flexDirection")] public FlexDirection FlexDirection { get; set; }
    [JsonPropertyName("size")] public double Size { get; set; }
    [JsonPropertyName("kind")] public string? Kind { get; set; }
    [JsonPropertyName("activeChildId")] public string? ActiveChildId { get; set; }
}

public sealed class LayoutTreeState
{
    public LayoutNode? RootNode { get; set; }
    public string? FocusedNodeId { get; set; }
    public string? MagnifiedNodeId { get; set; }

    public LayoutTreeState(LayoutNode? rootNode) => RootNode = rootNode;
}

public class LayoutTreeAction
{
    public LayoutTreeActionType Type { get; set; }
}

public sealed class LayoutTreeComputeMoveNodeAction : LayoutTreeAction
{
    public string? NodeId { get; set; }
    public string? NodeToMoveId { get; set; }
    public DropDirection? Direction { get; set; }

    public LayoutTreeComputeMoveNodeAction() => Type = LayoutTreeActionType.ComputeMove;
}

public sealed class LayoutTreeMoveNodeAction : LayoutTreeAction
{
    public int Index { get; set; }
    public string? ParentId { get; set; }
    public bool InsertAtRoot { get; set; }
    public LayoutNode? Node { get; set; }

    public LayoutTreeMoveNodeAction() => Type = LayoutTreeActionType.Move;
}

public sealed class LayoutTreeSwapNodeAction : LayoutTreeAction
{
    public string? Node1Id { get; set; }
    public string? Node2Id { get; set; }

    public LayoutTreeSwapNodeAction() => Type = LayoutTreeActionType.Swap;
}

public sealed class LayoutTreeInsertNodeAction : LayoutTreeAction
{
    public LayoutNode? Node { get; set; }
    public bool Magnified { get; set; }
    public bool Focused { get; set; }

    public LayoutTreeInsertNodeAction() => Type = LayoutTreeActionType.InsertNode;
}

public sealed class LayoutTreeInsertNodeAtIndexAction : LayoutTreeAction
{
    public LayoutNode? Node { get; set; }
    public List<int>? IndexArr { get; set; }
    public bool Magnified { get; set; }
    public bool Focused { get; set; }

    public LayoutTreeInsertNodeAtIndexAction() => Type = LayoutTreeActionType.InsertNodeAtIndex;
}

public sealed class LayoutTreeDeleteNodeAction : LayoutTreeAction
{
    public string? NodeId { get; set; }

    public LayoutTreeDeleteNodeAction() => Type = LayoutTreeActionType.DeleteNode;
}

public sealed class LayoutTreeReplaceNodeAction : LayoutTreeAction
{
    public string TargetNodeId { get; set; } = "";
    public LayoutNode NewNode { get; set; } = null!;
    public bool Focused { get; set; }

    public LayoutTreeReplaceNodeAction() => Type = LayoutTreeActionType.ReplaceNode;
}

public sealed class LayoutTreeSplitAction : LayoutTreeAction
{
    public string TargetNodeId { get; set; } = "";
    public LayoutNode NewNode { get; set; } = null!;
    public bool Before { get; set; }
    public bool Focused { get; set; }
}

public sealed record ResizeNodeOperation(string? NodeId, double Size);

public sealed class LayoutTreeResizeNodeAction : LayoutTreeAction
{
    public List<ResizeNodeOperation>? ResizeOperations { get; set; }

    public LayoutTreeResizeNodeAction() => Type = LayoutTreeActionType.ResizeNode;
}

public sealed class LayoutTreeStackAction : LayoutTreeAction
{
    public string? TargetNodeId { get; set; }
    public string? NodeId { get; set; }
    public bool? Focused { get; set; }

    public LayoutTreeStackAction() => Type = LayoutTreeActionType.Stack;
}

public sealed class LayoutTreeUnstackAction : LayoutTreeAction
{
    public string? NodeId { get; set; }
    public bool? Focused { get; set; }

    public LayoutTreeUnstackAction() => Type = LayoutTreeActionType.Unstack;
}

public sealed class LayoutTreeStackInsertAction : LayoutTreeAction
{
    public LayoutNode? Node { get; set; }
    public string? TargetNodeId { get; set; }
    public bool? Focused { get; set; }

    public LayoutTreeStackInsertAction() => Type = LayoutTreeActionType.StackInsert;
}

public sealed class LayoutTreeStackMoveAction : LayoutTreeAction
{
    public string? FromNodeId { get; set; }
    public string? ToNodeId { get; set; }
    public bool? Focused { get; set; }

    public LayoutTreeStackMoveAction() => Type = LayoutTreeActionType.StackMove;
}

public sealed class LayoutTreeStackReorderAction : LayoutTreeAction
{
    public string? NodeId { get; set; }
    public string? TargetNodeId { get; set; }

    public LayoutTreeStackReorderAction() => Type = LayoutTreeActionType.StackReorder;
}

public sealed class LayoutTreeFocusNodeAction : LayoutTreeAction
{
    public string? NodeId { get; set; }

    public LayoutTreeFocusNodeAction() => Type = LayoutTreeActionType.FocusNode;
}

public sealed class LayoutTreeMagnifyNodeToggleAction : LayoutTreeAction
{
    public string? NodeId { get; set; }

    public LayoutTreeMagnifyNodeToggleAction() => Type = LayoutTreeActionType.MagnifyNodeToggle;
}

public readonly record struct Dimensions(double Top, double Left, double Width, double Height);
