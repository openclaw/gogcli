export interface BotDetectionResult {
  isBot: boolean;
  botType: string | null;
}

type BotManagementInfo = {
  verifiedBot?: boolean;
  score?: number;
};

// Apple Private Relay IP ranges (simplified - real impl would use full list)
const APPLE_RELAY_PREFIXES = [
  '17.', // Apple corporate
  '104.28.', // Cloudflare for Apple
];

export function detectBot(
  userAgent: string,
  ip: string,
  timeSinceDeliveryMs: number | null,
  headers: Headers = new Headers(),
  botManagement?: BotManagementInfo
): BotDetectionResult {
  const hasAcceptHeader = headers.get('Accept') !== null;
  const hasRefererHeader = headers.get('Referer') !== null || headers.get('referer') !== null;

  // Known bot signals from Cloudflare
  if (botManagement?.verifiedBot) {
    return { isBot: true, botType: 'bot_managed' };
  }

  if (botManagement?.score !== undefined && botManagement.score < 20) {
    return { isBot: true, botType: 'low_bot_score' };
  }

  // Gmail Image Proxy = real human (Gmail proxies on their behalf)
  if (userAgent.includes('GoogleImageProxy')) {
    return { isBot: false, botType: 'gmail_proxy' };
  }

  if (!hasAcceptHeader && !hasRefererHeader) {
    return { isBot: true, botType: 'missing_headers' };
  }

  // Apple Mail Privacy Protection
  if (APPLE_RELAY_PREFIXES.some(prefix => ip.startsWith(prefix))) {
    return { isBot: true, botType: 'apple_mpp' };
  }

  // Missing or suspicious User-Agent
  if (!userAgent || userAgent === 'unknown' || userAgent.trim().length < 8) {
    return { isBot: true, botType: 'invalid_user_agent' };
  }

  // Outlook prefetch
  if (userAgent.includes('Outlook-iOS') ||
      userAgent.includes('Microsoft Outlook') ||
      userAgent.includes('ms-office')) {
    return { isBot: true, botType: 'outlook_prefetch' };
  }

  // Time-based detection: opens < 2 seconds after delivery are suspicious
  if (timeSinceDeliveryMs !== null && timeSinceDeliveryMs < 2000) {
    return { isBot: true, botType: 'prefetch' };
  }

  // Security scanners
  if (userAgent.includes('Barracuda') ||
      userAgent.includes('Symantec') ||
      userAgent.includes('Proofpoint')) {
    return { isBot: true, botType: 'security_scanner' };
  }

  return { isBot: false, botType: null };
}
