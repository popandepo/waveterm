// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
namespace WaveNative.Layout;

// Port of frontend/layout/lib/layoutNode.ts. Same behaviour, including the order of the steps: the TypeScript tests are the acceptance suite.
public static class LayoutNodes
{
    public const int DefaultMaxChildren = 5;

    public static FlexDirection Reverse(FlexDirection d) => d == FlexDirection.Row ? FlexDirection.Column : FlexDirection.Row;

    public static LayoutNode NewLayoutNode(FlexDirection? flexDirection = null, double? size = null, List<LayoutNode>? children = null, TabLayoutData? data = null)
    {
        var node = new LayoutNode
        {
            Id = Guid.NewGuid().ToString(),
            FlexDirection = flexDirection ?? FlexDirection.Row,
            Size = size ?? LayoutNode.DefaultNodeSize,
            Children = children,
            Data = data,
        };
        if (!ValidateNode(node)) throw new InvalidOperationException("Invalid node");
        return node;
    }

    public static void AddChildAt(LayoutNode node, int idx, params LayoutNode[] children)
    {
        if (children.Length == 0) return;
        if (node.Children == null || IsStackNode(node)) AddIntermediateNode(node);
        var toAdd = new List<LayoutNode>();
        foreach (var v in children)
        {
            if (IsStackNode(v))
            {
                if (v.FlexDirection == node.FlexDirection) v.FlexDirection = Reverse(node.FlexDirection);
                toAdd.Add(v);
            }
            else if (v.FlexDirection != node.FlexDirection) toAdd.Add(v);
            else if (v.Children != null) toAdd.AddRange(v.Children);
            else
            {
                v.FlexDirection = Reverse(node.FlexDirection);
                toAdd.Add(v);
            }
        }
        if (node.Children!.Count <= idx) node.Children.AddRange(toAdd);
        else if (idx >= 0) node.Children.InsertRange(idx, toAdd);
    }

    public static LayoutNode AddIntermediateNode(LayoutNode node)
    {
        LayoutNode intermediate;
        if (IsStackNode(node))
        {
            intermediate = NewLayoutNode(Reverse(node.FlexDirection), null, node.Children);
            intermediate.Kind = "stack";
            intermediate.ActiveChildId = node.ActiveChildId;
            node.Kind = null;
            node.ActiveChildId = null;
            node.Children = new List<LayoutNode> { intermediate };
        }
        else if (node.Data != null)
        {
            intermediate = NewLayoutNode(Reverse(node.FlexDirection), null, null, node.Data);
            node.Children = new List<LayoutNode> { intermediate };
            node.Data = null;
        }
        else
        {
            var inner = NewLayoutNode(node.FlexDirection, null, node.Children);
            intermediate = NewLayoutNode(Reverse(node.FlexDirection), null, new List<LayoutNode> { inner });
            node.Children = new List<LayoutNode> { intermediate };
        }
        (intermediate.Id, node.Id) = (node.Id, intermediate.Id);
        return intermediate;
    }

    public static bool IsStackNode(LayoutNode? node) => node?.Kind == "stack";

    public static LayoutNode NewStackNode(List<LayoutNode> leaves, string? activeChildId = null, double? size = null)
    {
        if (leaves.Count == 0 || leaves.Any(l => l.Children != null || l.Data == null))
            throw new InvalidOperationException("A stack needs one or more leaves");
        var stack = NewLayoutNode(FlexDirection.Row, size, leaves);
        stack.Kind = "stack";
        stack.ActiveChildId = leaves.Any(l => l.Id == activeChildId) ? activeChildId : leaves[0].Id;
        return stack;
    }

    public static void AddStackMember(LayoutNode stack, int idx, LayoutNode leaf, bool activate = false)
    {
        if (!IsStackNode(stack)) throw new InvalidOperationException("not a stack");
        if (leaf.Children != null || leaf.Data == null) throw new InvalidOperationException("only a leaf can be added to a stack");
        var at = idx < 0 || idx > stack.Children!.Count ? stack.Children.Count : idx;
        stack.Children.Insert(at, leaf);
        if (activate || string.IsNullOrEmpty(stack.ActiveChildId)) stack.ActiveChildId = leaf.Id;
    }

    public static void RemoveChild(LayoutNode parent, LayoutNode childToRemove, int startingIndex = 0)
    {
        if (parent.Children == null) return;
        var idx = startingIndex >= parent.Children.Count ? -1 : parent.Children.IndexOf(childToRemove, startingIndex);
        if (idx == -1) return;
        parent.Children.RemoveAt(idx);
    }

    public static LayoutNode? FindNode(LayoutNode? node, string? id)
    {
        if (node == null) return null;
        if (node.Id == id) return node;
        if (node.Children == null) return null;
        foreach (var child in node.Children)
        {
            var r = FindNode(child, id);
            if (r != null) return r;
        }
        return null;
    }

    public static LayoutNode? FindParent(LayoutNode node, string? id)
    {
        if (node.Id == id || node.Children == null) return null;
        foreach (var child in node.Children)
        {
            if (child.Id == id) return node;
            var r = FindParent(child, id);
            if (r != null) return r;
        }
        return null;
    }

    public static bool ValidateNode(LayoutNode node)
    {
        if ((node.Children == null) == (node.Data == null)) return false;
        if (node.Children?.Count == 0) return false;
        if (IsStackNode(node) && node.Children!.Any(c => c.Children != null || c.Data == null)) return false;
        return true;
    }

    public static List<string> OrphanLeafIds(LayoutNode? root, List<string>? tabBlockIds)
    {
        var orphans = new List<string>();
        if (root == null || tabBlockIds == null) return orphans;
        var live = new HashSet<string>(tabBlockIds);
        WalkNodes(root, n =>
        {
            if (!string.IsNullOrEmpty(n.Data?.BlockId) && !live.Contains(n.Data!.BlockId!)) orphans.Add(n.Id);
        });
        return orphans;
    }

    public static void WalkNodes(LayoutNode? node, Action<LayoutNode>? before = null, Action<LayoutNode>? after = null)
    {
        if (node == null) return;
        before?.Invoke(node);
        if (node.Children != null)
            foreach (var child in node.Children.ToList()) WalkNodes(child, before, after);
        after?.Invoke(node);
    }

    static LayoutNode? PruneHoles(LayoutNode? node)
    {
        if (node == null) return null;
        if (node.Children == null) return node;
        node.Children = node.Children.Select(PruneHoles).Where(c => c != null).Select(c => c!).ToList();
        return node.Children.Count == 0 ? null : node;
    }

    public static LayoutNode? BalanceNode(LayoutNode? node, Action<LayoutNode>? before = null, Action<LayoutNode>? after = null)
    {
        node = PruneHoles(node);
        WalkNodes(node,
            n =>
            {
                if (!ValidateNode(n)) throw new InvalidOperationException("Invalid node");
                if (n.Children != null)
                {
                    var rebuilt = new List<LayoutNode>();
                    foreach (var child in n.Children)
                    {
                        if (child.FlexDirection == n.FlexDirection) child.FlexDirection = Reverse(n.FlexDirection);
                        if (!IsStackNode(child) && child.Children?.Count == 1 && IsStackNode(child.Children[0]))
                        {
                            child.Children[0].Size = child.Size;
                            rebuilt.Add(child.Children[0]);
                            continue;
                        }
                        if (child.Children?.Count == 1 && child.Children[0].Children != null)
                        {
                            rebuilt.AddRange(child.Children[0].Children!);
                            continue;
                        }
                        if (child.Children?.Count == 0) continue;
                        rebuilt.Add(child);
                    }
                    n.Children = rebuilt;
                }
                before?.Invoke(n);
            },
            n =>
            {
                if (n.Children?.Count == 1 && n.Children[0].Children == null && !IsStackNode(n))
                {
                    n.Data = n.Children[0].Data;
                    n.Id = n.Children[0].Id;
                    n.Children = null;
                    n.Kind = null;
                    n.ActiveChildId = null;
                }
                else if (!IsStackNode(n) && n.Children?.Count == 1 && IsStackNode(n.Children[0]))
                {
                    var only = n.Children[0];
                    n.Kind = only.Kind;
                    n.ActiveChildId = only.ActiveChildId;
                    n.Id = only.Id;
                    n.Children = only.Children;
                }
                else if (IsStackNode(n) && n.Children?.Count > 0)
                {
                    if (!n.Children.Any(c => c.Id == n.ActiveChildId)) n.ActiveChildId = n.Children[0].Id;
                }
                after?.Invoke(n);
            });
        return node;
    }

    public readonly record struct InsertLocation(LayoutNode? Node, int Index);

    public static InsertLocation FindNextInsertLocation(LayoutNode node, int maxChildren = DefaultMaxChildren)
    {
        var loc = FindNextInsertLocationHelper(node, maxChildren, 1);
        return new InsertLocation(loc?.Node, loc?.Index ?? 0);
    }

    // Consumes indexArr (the TypeScript shifts it): the caller's list is emptied as the search goes down.
    public static InsertLocation? FindInsertLocationFromIndexArr(LayoutNode node, List<int> indexArr)
    {
        int Normalize(int index)
        {
            var len = node.Children?.Count ?? 1;
            var last = len - 1;
            if (index < 0) return len - Math.Max(index, -len);
            return Math.Min(index, last);
        }
        if (indexArr.Count == 0) return null;
        var first = indexArr[0];
        indexArr.RemoveAt(0);
        var next = Normalize(first);
        if (indexArr.Count == 0 || node.Children == null) return new InsertLocation(node, next);
        return FindInsertLocationFromIndexArr(node.Children[next], indexArr);
    }

    sealed record Loc(LayoutNode Node, int Index, int Depth);

    static Loc? FindNextInsertLocationHelper(LayoutNode? node, int maxChildren, int curDepth)
    {
        if (node == null) return null;
        if (node.Children == null || IsStackNode(node)) return new Loc(node, 1, curDepth);
        var locs = new List<Loc?>();
        if (node.Children.Count < maxChildren) locs.Add(new Loc(node, node.Children.Count, curDepth));
        foreach (var child in Enumerable.Reverse(node.Children)) locs.Add(FindNextInsertLocationHelper(child, maxChildren, curDepth + 1));
        return locs.Where(l => l != null).Select(l => l!).OrderBy(l => Math.Pow(l.Depth, l.Index + maxChildren)).FirstOrDefault();
    }

    public static double TotalChildrenSize(LayoutNode node) => node.Children?.Sum(c => c.Size) ?? 0;
}
