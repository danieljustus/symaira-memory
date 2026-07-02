import SwiftUI

extension Color {
    // Deep Warm Charcoal Obsidian
    public static let bgDark = Color(red: 13/255, green: 12/255, blue: 10/255)
    public static let bgDarker = Color(red: 7/255, green: 6/255, blue: 5/255)
    
    // Champagne Gold Theme
    public static let goldPrimary = Color(red: 229/255, green: 195/255, blue: 151/255)
    public static let goldSecondary = Color(red: 248/255, green: 230/255, blue: 205/255)
    public static let goldShadow = Color(red: 194/255, green: 153/255, blue: 101/255)
    
    // Warm Ice
    public static let icePrimary = Color(red: 238/255, green: 220/255, blue: 196/255)
    public static let iceSecondary = Color(red: 212/255, green: 178/255, blue: 133/255)
    public static let iceShadow = Color(red: 163/255, green: 128/255, blue: 84/255)
    
    // Typography
    public static let textPrimary = Color(red: 245/255, green: 244/255, blue: 240/255) // Warm Bone
    public static let textSecondary = Color(red: 181/255, green: 174/255, blue: 165/255) // Warm Sand Silver
    public static let textMuted = Color(red: 110/255, green: 104/255, blue: 96/255) // Soft warm gray
    
    // Glassmorphic backgrounds & borders
    public static let cardBackground = Color(red: 18/255, green: 17/255, blue: 14/255).opacity(0.65)
    public static let cardBackgroundHover = Color(red: 26/255, green: 24/255, blue: 20/255).opacity(0.8)
    public static let borderGlass = Color.white.opacity(0.05)
    public static let borderGlassHover = Color(red: 229/255, green: 195/255, blue: 151/255).opacity(0.18)
    
    // Brand Gradient accent
    public static let brandGradient = LinearGradient(
        colors: [.goldPrimary, .icePrimary],
        startPoint: .leading,
        endPoint: .trailing
    )
}
