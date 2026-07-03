import SwiftUI
@_exported import SymairaTheme

// This app historically used unprefixed Color statics; map them onto the
// shared brand tokens from symaira-appkit.
extension Color {
    public static let bgDark = SymairaTheme.bgDark
    public static let bgDarker = SymairaTheme.bgDarker

    public static let goldPrimary = SymairaTheme.goldPrimary
    public static let goldSecondary = SymairaTheme.goldSecondary
    public static let goldShadow = SymairaTheme.goldShadow

    public static let icePrimary = SymairaTheme.icePrimary
    public static let iceSecondary = SymairaTheme.iceSecondary

    public static let textPrimary = SymairaTheme.textPrimary
    public static let textSecondary = SymairaTheme.textSecondary
    public static let textMuted = SymairaTheme.textMuted

    public static let cardBackground = SymairaTheme.bgCard
    public static let cardBackgroundHover = SymairaTheme.bgCardHover

    // Memory-specific values that deviate from the shared tokens on purpose
    // (kept local for pixel-identical rendering; revisit in the hub).
    public static let borderGlass = Color.white.opacity(0.05)
    public static let borderGlassHover = SymairaTheme.goldPrimary.opacity(0.18)
}
