import SwiftUI

struct DashboardView: View {
    @EnvironmentObject var client: APIClient
    @State private var isPulsing = false
    
    var body: some View {
        ZStack {
            Color.bgDark.ignoresSafeArea()
            
            // Top Ambient Gold Glow
            RadialGradient(
                colors: [Color.goldPrimary.opacity(0.06), Color.clear],
                center: .top,
                startRadius: 0,
                endRadius: 500
            )
            .ignoresSafeArea()
            
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    // Header
                    HStack {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Symaira Memory Dashboard")
                                .font(.system(.title, design: .rounded))
                                .fontWeight(.bold)
                                .foregroundColor(.textPrimary)
                            Text("Offline-first semantic agent memory console")
                                .font(.subheadline)
                                .foregroundColor(.textSecondary)
                        }
                        
                        Spacer()
                        
                        // Connected Status Indicator
                        HStack(spacing: 8) {
                            statusLight
                            Text(connectionLabel)
                                .font(.system(size: 12, weight: .semibold, design: .rounded))
                                .foregroundColor(.textSecondary)
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 8)
                        .background(Color.bgDarker)
                        .cornerRadius(20)
                        .overlay(
                            RoundedRectangle(cornerRadius: 20)
                                .stroke(Color.borderGlass, lineWidth: 1)
                        )
                    }
                    .padding(.bottom, 8)
                    
                    // Status Box if not connected
                    if case let .failed(error) = client.connectionStatus {
                        HStack(spacing: 12) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundColor(.goldPrimary)
                                .font(.title)
                            VStack(alignment: .leading, spacing: 4) {
                                Text("Connection issue detected")
                                    .font(.headline)
                                    .foregroundColor(.textPrimary)
                                Text(error)
                                    .font(.subheadline)
                                    .foregroundColor(.textSecondary)
                            }
                        }
                        .padding()
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.goldPrimary.opacity(0.08))
                        .cornerRadius(12)
                        .overlay(
                            RoundedRectangle(cornerRadius: 12)
                                .stroke(Color.goldPrimary.opacity(0.2), lineWidth: 1)
                        )
                    }
                    
                    // Key Stats Cards
                    HStack(spacing: 16) {
                        StatCard(
                            title: "Total Memories",
                            value: "\(client.memories.count)",
                            icon: "brain.head.profile",
                            color: .goldPrimary,
                            isLoading: client.isFetching
                        )
                        
                        StatCard(
                            title: "Behavioral Rules",
                            value: "\(client.rules.count)",
                            icon: "doc.text.fill",
                            color: .icePrimary,
                            isLoading: client.isFetching
                        )
                    }
                    
                    // Scope Breakdown
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Memories by Scope")
                            .font(.headline)
                            .foregroundColor(.textPrimary)
                            .padding(.leading, 2)
                        
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 145))], spacing: 12) {
                            ScopeMiniCard(title: "Global", count: countForScope("global"), color: .purple)
                            ScopeMiniCard(title: "Project", count: countForScope("project"), color: .blue)
                            ScopeMiniCard(title: "Agent", count: countForScope("agent"), color: .green)
                            ScopeMiniCard(title: "User", count: countForScope("user"), color: .orange)
                            ScopeMiniCard(title: "Session", count: countForScope("session"), color: .gray)
                        }
                    }
                    
                    // Quick Actions Panel
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Quick Actions")
                            .font(.headline)
                            .foregroundColor(.textPrimary)
                            .padding(.leading, 2)
                        
                        HStack(spacing: 16) {
                            QuickActionBtn(title: "Sync Daemon", icon: "arrow.clockwise") {
                                Task {
                                    await client.checkStatus()
                                    await client.fetchMemories()
                                    await client.fetchRules()
                                }
                            }
                            
                            QuickActionBtn(title: "Launch Help", icon: "questionmark.circle") {
                                if let url = URL(string: "http://127.0.0.1:8989/api/status") {
                                    NSWorkspace.shared.open(url)
                                }
                            }
                        }
                    }
                }
                .padding(24)
            }
        }
        .onAppear {
            isPulsing = true
            Task {
                await client.checkStatus()
                await client.fetchMemories()
                await client.fetchRules()
            }
        }
    }
    
    private var connectionLabel: String {
        switch client.connectionStatus {
        case .disconnected: return "Disconnected"
        case .connecting: return "Checking connection..."
        case .connected(let version): return "Active daemon (v\(version))"
        case .failed: return "Daemon offline"
        }
    }
    
    @ViewBuilder
    private var statusLight: some View {
        switch client.connectionStatus {
        case .connected:
            Circle()
                .fill(Color.goldPrimary)
                .frame(width: 8, height: 8)
                .shadow(color: .goldPrimary, radius: 4)
                .scaleEffect(isPulsing ? 1.2 : 0.8)
                .animation(Animation.easeInOut(duration: 1.2).repeatForever(autoreverses: true), value: isPulsing)
        case .connecting:
            ProgressView()
                .controlSize(.mini)
        case .disconnected, .failed:
            Circle()
                .fill(Color.red)
                .frame(width: 8, height: 8)
                .shadow(color: .red, radius: 4)
        }
    }
    
    private func countForScope(_ scope: String) -> Int {
        return client.memories.filter { $0.scope.lowercased() == scope.lowercased() }.count
    }
}

struct StatCard: View {
    let title: String
    let value: String
    let icon: String
    let color: Color
    let isLoading: Bool
    
    @State private var isHovering = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: icon)
                    .font(.title2)
                    .foregroundColor(color)
                Spacer()
                if isLoading {
                    ProgressView().controlSize(.mini)
                }
            }
            
            VStack(alignment: .leading, spacing: 4) {
                Text(value)
                    .font(.system(.largeTitle, design: .rounded))
                    .fontWeight(.bold)
                    .foregroundColor(.textPrimary)
                Text(title)
                    .font(.subheadline)
                    .foregroundColor(.textSecondary)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(isHovering ? Color.cardBackgroundHover : Color.cardBackground)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(isHovering ? Color.borderGlassHover : Color.borderGlass, lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.3), radius: 6, x: 0, y: 3)
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.2)) {
                isHovering = hovering
            }
        }
    }
}

struct ScopeMiniCard: View {
    let title: String
    let count: Int
    let color: Color
    
    @State private var isHovering = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Circle()
                    .fill(color)
                    .frame(width: 8, height: 8)
                Text(title)
                    .font(.subheadline)
                    .fontWeight(.medium)
                    .foregroundColor(.textSecondary)
            }
            Text("\(count)")
                .font(.system(.title2, design: .rounded))
                .fontWeight(.bold)
                .foregroundColor(.textPrimary)
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(isHovering ? Color.cardBackgroundHover : Color.cardBackground)
        .cornerRadius(10)
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(isHovering ? Color.borderGlassHover : Color.borderGlass, lineWidth: 1)
        )
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.2)) {
                isHovering = hovering
            }
        }
    }
}

struct QuickActionBtn: View {
    let title: String
    let icon: String
    let action: () -> Void
    
    @State private var isHovering = false
    
    var body: some View {
        Button(action: action) {
            HStack(spacing: 8) {
                Image(systemName: icon)
                Text(title)
                    .font(.system(.body, design: .rounded))
                    .fontWeight(.semibold)
            }
            .padding(.horizontal, 18)
            .padding(.vertical, 10)
            .background(isHovering ? Color.goldPrimary.opacity(0.2) : Color.goldPrimary.opacity(0.08))
            .foregroundColor(.goldPrimary)
            .cornerRadius(8)
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(isHovering ? Color.goldPrimary.opacity(0.4) : Color.goldPrimary.opacity(0.18), lineWidth: 1)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.2)) {
                isHovering = hovering
            }
        }
    }
}
