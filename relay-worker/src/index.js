// Vibe Relay — Cloudflare Worker
// A lightweight URL discovery service for vibe-vcs.
// Servers publish their current tunnel URL here; daemons query to re-discover.
//
// Endpoints:
//   POST /publish              — server registers its URL (per-repo token required)
//   GET  /discover/:id?token=  — client looks up current URL (per-repo token required)
//   GET  /health               — health check
//
// Auth model: each repo generates its own random token at init time.
// The token is stored with the entry on first publish and must match on all
// subsequent publishes and discovers. Only linked clients know the token.

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    const corsHeaders = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type",
    };

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders });
    }

    if (path === "/publish" && request.method === "POST") {
      return handlePublish(request, env, corsHeaders);
    }

    if (path.startsWith("/discover/") && request.method === "GET") {
      const serverID = path.slice("/discover/".length);
      return handleDiscover(serverID, url, env, corsHeaders);
    }

    // DELETE /unpublish/:server_id?token=
    if (path.startsWith("/unpublish/") && request.method === "DELETE") {
      const serverID = path.slice("/unpublish/".length);
      return handleUnpublish(serverID, url, env, corsHeaders);
    }

    if (path === "/health") {
      return jsonResponse({ status: "ok" }, 200, corsHeaders);
    }

    return jsonResponse({ error: "not found" }, 404, corsHeaders);
  },
};

async function handlePublish(request, env, corsHeaders) {
  let body;
  try {
    body = await request.json();
  } catch {
    return jsonResponse({ error: "invalid json" }, 400, corsHeaders);
  }

  const { server_id, tunnel_url, token, lan_urls } = body;

  if (!server_id || !tunnel_url || !token) {
    return jsonResponse(
      { error: "server_id, tunnel_url, and token required" },
      400,
      corsHeaders
    );
  }

  // Per-repo auth: if this server_id already exists, token must match
  const existing = await env.RELAY_KV.get(`server:${server_id}`, "json");
  if (existing && existing.token !== token) {
    return jsonResponse({ error: "unauthorized: token mismatch" }, 401, corsHeaders);
  }

  const entry = {
    server_id,
    tunnel_url,
    lan_urls: lan_urls || [],
    token,          // stored but never returned to clients
    updated_at: new Date().toISOString(),
  };

  // 24h TTL — auto-expires if server stops publishing
  await env.RELAY_KV.put(`server:${server_id}`, JSON.stringify(entry), {
    expirationTtl: 86400,
  });

  return jsonResponse({ status: "ok" }, 200, corsHeaders);
}

async function handleDiscover(serverID, url, env, corsHeaders) {
  if (!serverID) {
    return jsonResponse({ error: "server_id required" }, 400, corsHeaders);
  }

  const reqToken = url.searchParams.get("token");
  if (!reqToken) {
    return jsonResponse({ error: "token required" }, 401, corsHeaders);
  }

  const entry = await env.RELAY_KV.get(`server:${serverID}`, "json");
  if (!entry) {
    return jsonResponse({ error: "not found" }, 404, corsHeaders);
  }

  if (entry.token !== reqToken) {
    return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
  }

  // Return the entry without the token
  const { token: _token, ...safe } = entry;
  return jsonResponse(safe, 200, corsHeaders);
}

async function handleUnpublish(serverID, url, env, corsHeaders) {
  if (!serverID) {
    return jsonResponse({ error: "server_id required" }, 400, corsHeaders);
  }
  const reqToken = url.searchParams.get("token");
  if (!reqToken) {
    return jsonResponse({ error: "token required" }, 401, corsHeaders);
  }
  const entry = await env.RELAY_KV.get(`server:${serverID}`, "json");
  if (!entry) {
    return jsonResponse({ error: "not found" }, 404, corsHeaders);
  }
  if (entry.token !== reqToken) {
    return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
  }
  await env.RELAY_KV.delete(`server:${serverID}`);
  return jsonResponse({ status: "ok" }, 200, corsHeaders);
}

function jsonResponse(data, status, extraHeaders = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...extraHeaders },
  });
}
