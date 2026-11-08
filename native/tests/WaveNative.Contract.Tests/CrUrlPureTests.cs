// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (f) url handling
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrUrlPureTests
{
    [Theory]
    [InlineData("HTTP://EXAMPLE.COM/a", "http://example.com/a")]
    [InlineData("http://Example.Com/AbC", "http://example.com/AbC")]
    [InlineData("http://example.com:80/", "http://example.com/")]
    [InlineData("http://example.com:80", "http://example.com/")]
    [InlineData("https://example.com:443/x", "https://example.com/x")]
    [InlineData("https://example.com:8443/x", "https://example.com:8443/x")]
    [InlineData("http://example.com:8080/x", "http://example.com:8080/x")]
    [InlineData("http://example.com:443/x", "http://example.com:443/x")]
    [InlineData("https://example.com:80/x", "https://example.com:80/x")]
    [InlineData("http://example.com", "http://example.com/")]
    [InlineData("https://example.com", "https://example.com/")]
    [InlineData("http://example.com/a?x=1", "http://example.com/a?x=1")]
    [InlineData("http://example.com/a?X=Y#Frag", "http://example.com/a?X=Y#Frag")]
    [InlineData("http://example.com/#f", "http://example.com/#f")]
    [InlineData("http://example.com?x=1", "http://example.com/?x=1")]
    public void NormalizeUrl_follows_the_4_5_rules(string input, string expected)
    {
        Assert.Equal(expected, Validators.NormalizeUrl(input));
    }

    [Fact]
    public void NormalizeUrl_keeps_the_query_so_different_queries_are_different_pages()
    {
        Assert.NotEqual(Validators.NormalizeUrl("http://a.test/?a=1"), Validators.NormalizeUrl("http://a.test/?a=2"));
        Assert.NotEqual(Validators.NormalizeUrl("http://a.test/p"), Validators.NormalizeUrl("http://a.test/p#x"));
        Assert.NotEqual(Validators.NormalizeUrl("http://a.test/p"), Validators.NormalizeUrl("http://a.test/q"));
        Assert.NotEqual(Validators.NormalizeUrl("http://a.test/"), Validators.NormalizeUrl("https://a.test/"));
    }

    [Fact]
    public void NormalizeUrl_makes_the_equal_forms_equal()
    {
        Assert.Equal(Validators.NormalizeUrl("HTTP://A.TEST:80"), Validators.NormalizeUrl("http://a.test/"));
    }

    [Fact]
    public void NormalizeUrl_of_a_file_url_keeps_the_path()
    {
        Assert.Equal(Validators.NormalizeUrl("file:///C:/x/y.html"), Validators.NormalizeUrl("FILE:///C:/x/y.html"));
        Assert.NotEqual(Validators.NormalizeUrl("file:///C:/x/y.html"), Validators.NormalizeUrl("file:///C:/x/z.html"));
        Assert.EndsWith("C:/x/y.html", Validators.NormalizeUrl("file:///C:/x/y.html"));
    }

    [Theory]
    [InlineData("http://example.com/p?token=secret", "http://example.com/p")]
    [InlineData("http://example.com/p#secret", "http://example.com/p")]
    [InlineData("http://u:secret@example.com/p", "http://example.com/p")]
    [InlineData("http://secret@example.com/p", "http://example.com/p")]
    [InlineData("http://u:p@Example.COM:8080/a/b?t=secret#f", "http://example.com:8080/a/b")]
    [InlineData("HTTPS://EXAMPLE.com:443/a", "https://example.com/a")]
    [InlineData("http://example.com:80/a", "http://example.com/a")]
    [InlineData("http://example.com", "http://example.com/")]
    [InlineData("https://example.com:8443", "https://example.com:8443/")]
    [InlineData("http://example.com/a/b/c", "http://example.com/a/b/c")]
    public void RedactUrl_drops_user_info_query_and_fragment(string input, string expected)
    {
        var redacted = Validators.RedactUrl(input);
        Assert.Equal(expected, redacted);
        Assert.DoesNotContain("secret", redacted);
    }

    [Fact]
    public void RedactUrl_of_something_that_is_not_a_url_is_empty()
    {
        Assert.Equal("", Validators.RedactUrl("not a url"));
        Assert.Equal("", Validators.RedactUrl(""));
    }

    [Theory]
    [InlineData("http://example.com/")]
    [InlineData("https://example.com/a?b=1")]
    [InlineData("HTTP://EXAMPLE.COM")]
    [InlineData("file:///C:/x/y.html")]
    public void Url_accepts_http_https_and_file(string url)
    {
        Assert.Equal(url, Validators.Url(url));
    }

    [Theory]
    [InlineData("javascript:alert(1)")]
    [InlineData("JavaScript:alert(1)")]
    [InlineData("data:text/html,hi")]
    [InlineData("ftp://example.com/x")]
    [InlineData("mailto:a@b.c")]
    [InlineData("about:blank")]
    [InlineData("example.com")]
    [InlineData("plain text")]
    [InlineData("//example.com/x")]
    [InlineData("")]
    public void Url_refuses_everything_else_with_usage(string url)
    {
        var e = Assert.Throws<CliError>(() => Validators.Url(url));
        Assert.Equal(2, e.Code);
    }

    [Theory]
    [InlineData("C:\\temp\\x.html")]
    [InlineData("C:/temp/x.html")]
    public void Url_refuses_a_bare_windows_path_because_it_is_not_a_file_url(string url)
    {
        var e = Assert.Throws<CliError>(() => Validators.Url(url));
        Assert.Equal(2, e.Code);
    }
}
