package config

const VERSION = "v1.3.0"

const HEADER = `
  ▗▄▄▄ ▗▖  ▗▖ ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖
  ▐▌  █▐▛▚▖▐▌▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █    █   ▝▚▞▘
  ▐▌  █▐▌ ▝▜▌ ▝▀▚▖▐▛▀▜▌▐▌ ▝▜▌  █    █    ▐▌
  ▐▙▄▄▀▐▌  ▐▌▗▄▄▞▘▐▌ ▐▌▐▌  ▐▌▗▄█▄▖  █    ▐▌
`

const DEFAULT_TEMPLATE = `
# <FQDN>                    <EXPECTED-RESULT>

# Multiple A records
cr.yp.to                    A=131.193.32.108 A=131.193.32.109

# These A & CNAME records are expected:
mbc.group.stanford.edu      CNAME=web.stanford.edu. A=171.67.215.200
wiki.debian.org             CNAME=wilder.debian.org. A=*

# # valid TLD, no records: SERVFAIL
# dnssec-failed.org           SERVFAIL # disabled, because non-dnssec-aware resolvers will fail here

# be flexible here, some servers return NOERROR (with no records),
# some timeout, some return formerr, anyway, we just want to ensure
# here that server didn't put an IP that shouldn't exist:
invalid.com                 SERVFAIL || NOERROR || TIMEOUT || FORMERR 

# # invalid TLD - NXDOMAIN is expected:
dn05jq2u.fr                 NXDOMAIN

# Single A record expected:
bet365.com                  A=5.226.17*
lists.isc.org               A=149.20.*
#www-78-46-204-247.sslip.io  A=78.46.204.247 # unreliable, sometimes 9.9.9.9 fails on it
app-c0a801fb.nip.io A=192.168.1.251
retro.localtest.me          A=127.0.0.1

algolia.net                 A=103.254.154.6 A=149.202.84.123 A=*

# PS: Beware of geo-located domains for reliable results !
`
