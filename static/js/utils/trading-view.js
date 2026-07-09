export function createTradingViewLink(symbol) {
    const displayTicker = `${symbol}.P`;
    const tradingViewSymbol = `BINANCE:${displayTicker}`;

    return {
        displayTicker,
        tradingViewSymbol,
        chartUrl: `https://www.tradingview.com/chart/?symbol=${encodeURIComponent(tradingViewSymbol)}`,
    };
}

export function createTradingViewEmbedUrl(symbol) {
    const params = new URLSearchParams({
        symbol,
        interval: "15",
        symboledit: "1",
        saveimage: "0",
        toolbarbg: "f1f3f6",
        studies: "[]",
        hideideas: "1",
        theme: "dark",
        style: "1",
        timezone: "Etc/UTC",
        hidevolume: "1",
        withdateranges: "0",
        allow_symbol_change: "1",
        locale: "ru",
    });

    return `https://www.tradingview.com/widgetembed/?${params.toString()}`;
}
