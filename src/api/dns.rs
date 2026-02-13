use std::io;
use std::net::{SocketAddr, UdpSocket};

/// Custom DNS resolver that queries 1.1.1.1:53 over IPv4.
/// Fixes DNS resolution on Android/Termux where the system resolver
/// tries IPv6 localhost ([::1]:53) which often doesn't work.
pub struct CloudflareDns;

impl ureq::Resolver for CloudflareDns {
    fn resolve(&self, netloc: &str) -> io::Result<Vec<SocketAddr>> {
        // netloc is "host:port"
        let (host, port) = match netloc.rsplit_once(':') {
            Some((h, p)) => (h, p.parse::<u16>().unwrap_or(443)),
            None => (netloc, 443),
        };

        // If it's already an IP address, return directly
        if let Ok(ip) = host.parse::<std::net::Ipv4Addr>() {
            return Ok(vec![SocketAddr::new(ip.into(), port)]);
        }
        if let Ok(ip) = host.parse::<std::net::Ipv6Addr>() {
            return Ok(vec![SocketAddr::new(ip.into(), port)]);
        }

        // Try system resolver first, filter to IPv4
        if let Ok(addrs) = std::net::ToSocketAddrs::to_socket_addrs(&netloc) {
            let v4: Vec<SocketAddr> = addrs.filter(|a| a.is_ipv4()).collect();
            if !v4.is_empty() {
                return Ok(v4);
            }
        }

        // Fallback: query 1.1.1.1 directly
        let ips = dns_query_a(host)?;
        if ips.is_empty() {
            return Err(io::Error::new(
                io::ErrorKind::Other,
                format!("DNS: no A records for {}", host),
            ));
        }

        Ok(ips
            .into_iter()
            .map(|ip| SocketAddr::new(ip.into(), port))
            .collect())
    }
}

/// Send a DNS A query to 1.1.1.1:53 and parse IPv4 addresses from the response.
fn dns_query_a(domain: &str) -> io::Result<Vec<std::net::Ipv4Addr>> {
    let mut packet = Vec::with_capacity(64);

    // Header: ID=0xABCD, flags=0x0100 (standard query, recursion desired)
    // QDCOUNT=1, ANCOUNT=0, NSCOUNT=0, ARCOUNT=0
    packet.extend_from_slice(&[0xAB, 0xCD, 0x01, 0x00]);
    packet.extend_from_slice(&[0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);

    // Question: encode domain name as labels
    for label in domain.split('.') {
        packet.push(label.len() as u8);
        packet.extend_from_slice(label.as_bytes());
    }
    packet.push(0); // root label

    // QTYPE=A (1), QCLASS=IN (1)
    packet.extend_from_slice(&[0x00, 0x01, 0x00, 0x01]);

    // Send to Cloudflare DNS
    let sock = UdpSocket::bind("0.0.0.0:0")?;
    sock.set_read_timeout(Some(std::time::Duration::from_secs(5)))?;
    sock.send_to(&packet, "1.1.1.1:53")?;

    let mut buf = [0u8; 512];
    let n = sock.recv(&mut buf)?;
    if n < 12 {
        return Err(io::Error::new(io::ErrorKind::Other, "DNS response too short"));
    }

    // Parse answer count from header
    let ancount = u16::from_be_bytes([buf[6], buf[7]]) as usize;

    // Skip header (12 bytes) and question section
    let mut pos = 12;
    // Skip QNAME
    while pos < n && buf[pos] != 0 {
        pos += buf[pos] as usize + 1;
    }
    pos += 1; // null terminator
    pos += 4; // QTYPE + QCLASS

    // Parse answers
    let mut addrs = Vec::new();
    for _ in 0..ancount {
        if pos + 12 > n {
            break;
        }

        // Skip NAME (may be compressed pointer or labels)
        if buf[pos] & 0xC0 == 0xC0 {
            pos += 2; // compressed pointer
        } else {
            while pos < n && buf[pos] != 0 {
                pos += buf[pos] as usize + 1;
            }
            pos += 1;
        }

        if pos + 10 > n {
            break;
        }

        let rtype = u16::from_be_bytes([buf[pos], buf[pos + 1]]);
        let rdlength = u16::from_be_bytes([buf[pos + 8], buf[pos + 9]]) as usize;
        pos += 10;

        if pos + rdlength > n {
            break;
        }

        // A record: type=1, rdlength=4
        if rtype == 1 && rdlength == 4 {
            addrs.push(std::net::Ipv4Addr::new(
                buf[pos],
                buf[pos + 1],
                buf[pos + 2],
                buf[pos + 3],
            ));
        }

        pos += rdlength;
    }

    Ok(addrs)
}
