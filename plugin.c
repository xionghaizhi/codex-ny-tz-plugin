
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <time.h>

#if defined(_WIN32)
#define CLIPROXY_EXPORT __declspec(dllexport)
#else
#define CLIPROXY_EXPORT __attribute__((visibility("default")))
#endif

#define ABI_VERSION 1

typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_plugin_call_fn)(const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;
typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	void* call;
	void* free_buffer;
} cliproxy_host_api;

static const char *B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static void write_mem(cliproxy_buffer* response, const char* text, size_t len) {
	if (response == NULL) return;
	void* ptr = malloc(len ? len : 1);
	if (!ptr) { response->ptr=NULL; response->len=0; return; }
	if (len) memcpy(ptr, text, len);
	response->ptr = ptr;
	response->len = len;
}

static void write_str(cliproxy_buffer* response, const char* text) {
	write_mem(response, text, text ? strlen(text) : 0);
}

static int b64val(int c) {
	if (c>='A'&&c<='Z') return c-'A';
	if (c>='a'&&c<='z') return c-'a'+26;
	if (c>='0'&&c<='9') return c-'0'+52;
	if (c=='+') return 62;
	if (c=='/') return 63;
	return -1;
}

static unsigned char *b64_decode(const char *in, size_t inlen, size_t *outlen) {
	size_t cap = inlen/4*3 + 4;
	unsigned char *out = malloc(cap);
	if (!out) return NULL;
	size_t i=0, o=0;
	int val=0, valb=-8;
	for (; i<inlen; i++) {
		unsigned char c = (unsigned char)in[i];
		if (c=='=' || c=='\n' || c=='\r' || c==' ') continue;
		int d = b64val(c);
		if (d<0) continue;
		val = (val<<6) + d;
		valb += 6;
		if (valb >= 0) {
			out[o++] = (unsigned char)((val>>valb)&0xFF);
			valb -= 8;
		}
	}
	*outlen = o;
	return out;
}

static char *b64_encode(const unsigned char *in, size_t inlen, size_t *outlen) {
	size_t cap = (inlen+2)/3*4 + 1;
	char *out = malloc(cap);
	if (!out) return NULL;
	size_t i=0, o=0;
	for (; i+2<inlen; i+=3) {
		out[o++]=B64[in[i]>>2];
		out[o++]=B64[((in[i]&3)<<4)|(in[i+1]>>4)];
		out[o++]=B64[((in[i+1]&15)<<2)|(in[i+2]>>6)];
		out[o++]=B64[in[i+2]&63];
	}
	if (i<inlen) {
		out[o++]=B64[in[i]>>2];
		if (i+1<inlen) {
			out[o++]=B64[((in[i]&3)<<4)|(in[i+1]>>4)];
			out[o++]=B64[((in[i+1]&15)<<2)];
			out[o++]='=';
		} else {
			out[o++]=B64[(in[i]&3)<<4];
			out[o++]='=';
			out[o++]='=';
		}
	}
	out[o]=0;
	*outlen=o;
	return out;
}

static int sunday_of_month(int year, int month, int nth) {
	/* month 1-12, nth=1 first sunday, nth=2 second sunday */
	struct tm t;
	memset(&t, 0, sizeof(t));
	t.tm_year = year-1900;
	t.tm_mon = month-1;
	t.tm_mday = 1;
	t.tm_hour = 12;
	time_t ts = timegm(&t);
	if (ts == (time_t)-1) return 1;
	gmtime_r(&ts, &t);
	int w = t.tm_wday; /* 0 sunday */
	int first = 1 + (7-w)%7;
	return first + (nth-1)*7;
}

static int ny_is_dst(int year, int yday, int hour_utc) {
	/* US Eastern DST: 2nd Sunday March 07:00 UTC to 1st Sunday November 06:00 UTC roughly.
	   Local 02:00 EST/EDT. Use: start 2nd Sunday March 07:00 UTC, end 1st Sunday Nov 06:00 UTC. */
	int start_d = sunday_of_month(year, 3, 2);
	int end_d = sunday_of_month(year, 11, 1);
	/* convert to 0-based yday */
	struct tm t; memset(&t,0,sizeof(t));
	t.tm_year=year-1900; t.tm_mon=2; t.tm_mday=start_d; t.tm_hour=12;
	time_t ts=timegm(&t); gmtime_r(&ts,&t); int start_yd=t.tm_yday;
	t.tm_mon=10; t.tm_mday=end_d; ts=timegm(&t); gmtime_r(&ts,&t); int end_yd=t.tm_yday;
	if (yday > start_yd && yday < end_yd) return 1;
	if (yday == start_yd && hour_utc >= 7) return 1;
	if (yday == end_yd && hour_utc < 6) return 1;
	return 0;
}

static void ny_ymd(char *buf, size_t n) {
	time_t now = time(NULL);
	struct tm utc;
	gmtime_r(&now, &utc);
	int dst = ny_is_dst(utc.tm_year+1900, utc.tm_yday, utc.tm_hour);
	now -= (dst ? 4 : 5) * 3600;
	gmtime_r(&now, &utc);
	snprintf(buf, n, "%04d-%02d-%02d", utc.tm_year+1900, utc.tm_mon+1, utc.tm_mday);
}

static int replace_span(char **ps, size_t *plen, const char *open, const char *close, const char *val) {
	char *s = *ps;
	size_t len = *plen;
	size_t ol = strlen(open), cl = strlen(close), vl = strlen(val);
	int changed = 0;
	size_t i = 0;
	while (i + ol + cl <= len) {
		if (memcmp(s+i, open, ol) != 0) { i++; continue; }
		char *e = NULL;
		for (size_t j=i+ol; j+cl<=len; j++) {
			if (memcmp(s+j, close, cl)==0) { e = s+j; break; }
		}
		if (!e) break;
		size_t old = (size_t)(e - (s+i)) + cl;
		size_t neu = ol + vl + cl;
		size_t nlen = len - old + neu;
		char *ns = malloc(nlen+1);
		if (!ns) return changed;
		memcpy(ns, s, i);
		memcpy(ns+i, open, ol);
		memcpy(ns+i+ol, val, vl);
		memcpy(ns+i+ol+vl, close, cl);
		memcpy(ns+i+neu, e+cl, len - (size_t)(e+cl - s));
		ns[nlen]=0;
		free(s);
		s = ns; len = nlen; *ps = s; *plen = len;
		i += neu;
		changed = 1;
	}
	return changed;
}

static unsigned char *rewrite_body(unsigned char *in, size_t inlen, size_t *outlen) {
	char *s = malloc(inlen+1);
	if (!s) return NULL;
	memcpy(s, in, inlen);
	s[inlen]=0;
	size_t len = inlen;
	char date[16];
	ny_ymd(date, sizeof(date));
	replace_span(&s, &len, "<timezone>", "</timezone>", "America/New_York");
	replace_span(&s, &len, "\\u003ctimezone\\u003e", "\\u003c/timezone\\u003e", "America/New_York");
	replace_span(&s, &len, "<current_date>", "</current_date>", date);
	replace_span(&s, &len, "\\u003ccurrent_date\\u003e", "\\u003c/current_date\\u003e", date);
	*outlen = len;
	return (unsigned char*)s;
}

static const char *find_body_b64(const char *json, size_t n, size_t *blen) {
	const char *key = "\"Body\":\"";
	const char *p = NULL;
	for (size_t i=0; i+8<n; i++) {
		if (memcmp(json+i, key, 8)==0) { p=json+i+8; break; }
	}
	if (!p) return NULL;
	const char *e = p;
	const char *end = json+n;
	while (e<end && *e!='"') {
		if (*e=='\\' && e+1<end) e+=2; else e++;
	}
	*blen = (size_t)(e-p);
	return p;
}

static char *json_escape_b64(const char *b64, size_t n, size_t *outn) {
	/* base64 has no quotes; copy as-is */
	char *o = malloc(n+1);
	if (!o) return NULL;
	memcpy(o, b64, n);
	o[n]=0;
	*outn=n;
	return o;
}

static int plugin_call(const char *method, const uint8_t *request, size_t request_len, cliproxy_buffer *response) {
	if (response) { response->ptr=NULL; response->len=0; }
	if (!method) {
		write_str(response, "{\"ok\":false,\"error\":{\"code\":\"invalid_method\",\"message\":\"method is required\"}}");
		return 1;
	}
	if (strcmp(method, "plugin.register")==0 || strcmp(method, "plugin.reconfigure")==0) {
		write_str(response,
			"{\"ok\":true,\"result\":{\"schema_version\":1,\"metadata\":{\"Name\":\"codex-ny-timezone\",\"Version\":\"1.0.0\",\"Author\":\"local\",\"GitHubRepository\":\"https://github.com/router-for-me/CLIProxyAPI\",\"Logo\":\"https://github.com/router-for-me/CLIProxyAPI\",\"ConfigFields\":[{\"Name\":\"timezone\",\"Type\":\"string\",\"Description\":\"IANA timezone\"}]},\"capabilities\":{\"request_normalizer\":true}}}");
		return 0;
	}
	if (strcmp(method, "request.normalize")==0) {
		size_t blen=0;
		const char *b64 = find_body_b64((const char*)request, request_len, &blen);
		if (!b64) {
			write_str(response, "{\"ok\":true,\"result\":{\"Body\":null}}");
			return 0;
		}
		size_t rawlen=0;
		unsigned char *raw = b64_decode(b64, blen, &rawlen);
		if (!raw) {
			write_str(response, "{\"ok\":false,\"error\":{\"code\":\"b64\",\"message\":\"decode failed\"}}");
			return 1;
		}
		size_t nlen=0;
		unsigned char *neu = rewrite_body(raw, rawlen, &nlen);
		free(raw);
		if (!neu) {
			write_str(response, "{\"ok\":false,\"error\":{\"code\":\"rewrite\",\"message\":\"rewrite failed\"}}");
			return 1;
		}
		size_t elen=0;
		char *enc = b64_encode(neu, nlen, &elen);
		free(neu);
		if (!enc) {
			write_str(response, "{\"ok\":false,\"error\":{\"code\":\"b64\",\"message\":\"encode failed\"}}");
			return 1;
		}
		const char *prefix = "{\"ok\":true,\"result\":{\"Body\":\"";
		const char *suffix = "\"}}";
		size_t plen = strlen(prefix)+elen+strlen(suffix);
		char *out = malloc(plen+1);
		if (!out) { free(enc); write_str(response, "{\"ok\":false}"); return 1; }
		memcpy(out, prefix, strlen(prefix));
		memcpy(out+strlen(prefix), enc, elen);
		memcpy(out+strlen(prefix)+elen, suffix, strlen(suffix)+1);
		free(enc);
		write_mem(response, out, plen);
		free(out);
		return 0;
	}
	write_str(response, "{\"ok\":false,\"error\":{\"code\":\"unknown_method\",\"message\":\"unknown method\"}}");
	return 0;
}

static void plugin_free(void* ptr, size_t len) { (void)len; free(ptr); }
static void plugin_shutdown(void) {}

CLIPROXY_EXPORT int cliproxy_plugin_init(const cliproxy_host_api* host, cliproxy_plugin_api* plugin) {
	(void)host;
	if (!plugin) return 1;
	plugin->abi_version = ABI_VERSION;
	plugin->call = plugin_call;
	plugin->free_buffer = plugin_free;
	plugin->shutdown = plugin_shutdown;
	return 0;
}
