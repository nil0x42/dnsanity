<h1 align="center">DNSanity :dart:</h1>

<h3 align="center">
    Quickly validate DNS servers at scale
    <a href="https://twitter.com/intent/tweet?text=DNSanity%3A%20validate%20massive%20lists%20of%20DNS%20resolvers%20at%20scale%20%28for%20recon%20%26%20DNS%20bruteforcing%29%20-%20by%20%40nil0x42&url=https://github.com/nil0x42/dnsanity">
      <img src="https://img.shields.io/twitter/url?label=tweet&logo=twitter&style=social&url=http%3A%2F%2F0" alt="tweet">
    </a>
</h3>
<br>

<p align="center">
  <a href="https://twitter.com/intent/follow?screen_name=nil0x42" target="_blank">
    <img src="https://img.shields.io/twitter/follow/nil0x42.svg?logo=twitter" akt="follow on twitter">
  </a>
</p>

<div align="center">
  <sub>
    Created by
    <a href="https://twitter.com/nil0x42">nil0x42</a> and
    <a href="https://github.com/nil0x42/dnsanity/graphs/contributors">contributors</a>
  </sub>
</div>

<br>

* * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * *

<img align="right" src=".github/images/demo.gif" width="60%"/>


### :book: Overview

**DNSanity** is a fast DNS resolvers validator, offering deep **customization**
and reliable **concurrency**.

If you want to validate massive lists with speed and precision, we have you covered !

- **Blazing-Fast**: Test thousand servers in parallel, with **global & per-server rate-limiting**.  
- **Flexible**: Easily write your own template for custom validation.  
- **Reliable**: Automatic template re-validation before every usage.  

<br>

### :arrows_clockwise: Workflow

**Template Validation (step 1/2)**  
Make sure template is still valid, matching it against trusted servers.

**Servers Sanitization (step 2/2)**  
For each server, every template test is checked.
If mismatches exceed threshold, server is dropped. Undropped
servers are considered valid.

<br>

### :bulb: Quick start

```bash
go install github.com/nil0x42/dnsanity@latest   # go 1.22+ recommended
dnsanity --help                                 # show help
dnsanity -list "untrustedDNS.txt" -o "out.txt"  # basic usage
```

<br>

### :card_index: Custom template

```bash
# <FQDN>             <EXPECTED-RESULT>                 <COMMENT>
cr.yp.to             A=131.193.32.108 A=131.193.32.109 # two specific A records
wiki.debian.org      A=* CNAME=wilder.debian.org.      # specific CNAME with any A record
dn05jq2u.fr          NXDOMAIN                          # invalid TLD: NXDOMAIN
dnssec-failed.org    SERVFAIL                          # valid TLD & no records: SERVFAIL
lists.isc.org        A=149.20.*                        # A record matching pattern
app-c0a801fb.nip.io  A=192.168.1.251                   # specific single A record
retro.localtest.me   A=127.0.0.1                       # specific single A record
```
A template test *(line)* defines what a domain must return when resolved by a DNS server.
Create your template, and use it with `dnsanity -template /path/to/template.txt`  


<br>

### :mag: Options

<img src=".github/images/help.png">

### :factory: Under the Hood

**DNSanity** aims for maximum speed without sacrificing reliability
or risking blacklisting. Here’s the core approach:

- **Trusted Validation**  
  Before checking your untrusted servers, DNSanity verifies the **template**
  itself against trusted resolvers (e.g., `8.8.8.8`, `1.1.1.1`).
  This ensures your template is valid and consistent.
- **Test-by-Test Concurrency**  
  For each untrusted server, DNSanity runs tests sequentially in
  an efficient pipeline. Once a server accumulates more mismatches than
  `-max-mismatches` *(default 0)*, it’s dropped immediately,
  saving time & bandwidth.
- **Per-Server Rate Limit**  
  Use `-ratelimit` so you don’t overload any single DNS server.
  This is especially helpful for fragile networks or for preventing
  blacklisting on public resolvers.
- **Timeout & Retries**  
  If a query doesn’t reply within `-timeout` seconds, it fails.
  If `-max-attempts` is greater than 1, DNSanity can retry,
  up to the specified limit.

<br>

### :information_source: Additional Tips

- **Craft a Thorough Template**  
  A varied template (involving A, CNAME, NXDOMAIN, and wildcard matches)
  quickly exposes shady or broken resolvers.
- **Geo-Located Domains**  
  Beware that some domains (e.g., google.com) may return different IP addresses
  based on location. This might cause expected results to mismatch.
- **Fine-tune template validation step**
  `-trusted-*` flags allow fine-tuning specific limits for this step, which
  uses trusted server list (use `--help` for details)

<br>

### :star: Acknowledgments

- **[dnsvalidator](https://github.com/vortexau/dnsvalidator)** – for the original concept of verifying DNS resolvers.  
- **[dnsx](https://github.com/projectdiscovery/dnsx)** – inspiration for a fast, multi-purpose DNS toolkit.  
- **[miekg/dns](https://github.com/miekg/dns)** – the Go library powering DNSanity queries under the hood.

---

**Happy Recon & Hacking!**
