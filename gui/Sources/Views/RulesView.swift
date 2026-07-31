import SwiftUI

struct RulesView: View {
    @EnvironmentObject var client: APIClient
    @State private var selectedScope = "all"
    
    let scopes = ["all", "global", "project", "agent", "user"]
    
    @State private var isRefreshHovered = false
    
    var body: some View {
        ZStack {
            Color.bgDark.ignoresSafeArea()
            
            // Top Ambient Gold Glow
            RadialGradient(
                colors: [Color.goldPrimary.opacity(0.04), Color.clear],
                center: .top,
                startRadius: 0,
                endRadius: 400
            )
            .ignoresSafeArea()
            
            VStack(spacing: 0) {
                // Header toolbar
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Behavioral Prompt Rules")
                            .symairaText(.heading)
                            .fontWeight(.bold)
                            .foregroundColor(.textPrimary)
                        Text("Procedural rules configured for AI agent execution contexts.")
                            .symairaText(.caption)
                            .foregroundColor(.textSecondary)
                    }
                    
                    Spacer()
                    
                    Button {
                        Task { await client.fetchRules(scope: selectedScope) }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .foregroundColor(.goldPrimary)
                    }
                    .buttonStyle(.plain)
                    .padding(8)
                    .background(isRefreshHovered ? Color.goldPrimary.opacity(0.15) : Color.goldPrimary.opacity(0.06))
                    .cornerRadius(8)
                    .overlay(
                        RoundedRectangle(cornerRadius: 8)
                            .stroke(Color.goldPrimary.opacity(0.18), lineWidth: 1)
                    )
                    .onHover { hovering in
                        isRefreshHovered = hovering
                    }
                }
                .padding()
                .background(Color.bgDarker.opacity(0.3))
                
                Divider()
                    .background(Color.borderGlass)
                
                // Scope Picker
                HStack {
                    ForEach(scopes, id: \.self) { scope in
                        Button {
                            selectedScope = scope
                            Task { await client.fetchRules(scope: selectedScope) }
                        } label: {
                            Text(scope.capitalized)
                                .symairaText(.body)
.fontWeight(selectedScope == scope ? .semibold : .regular)
                                .padding(.horizontal, 14)
                                .padding(.vertical, 7)
                                .background(selectedScope == scope ? Color.goldPrimary.opacity(0.12) : Color.clear)
                                .foregroundColor(selectedScope == scope ? .goldPrimary : .textSecondary)
                                .cornerRadius(6)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 6)
                                        .stroke(selectedScope == scope ? Color.goldPrimary.opacity(0.2) : Color.clear, lineWidth: 1)
                                )
                        }
                        .buttonStyle(.plain)
                    }
                    Spacer()
                }
                .padding(.horizontal)
                .padding(.vertical, 10)
                .background(Color.bgDarker.opacity(0.15))
                
                Divider()
                    .background(Color.borderGlass)
                
                // Content List
                if client.isFetching && client.rules.isEmpty {
                    VStack {
                        Spacer()
                        ProgressView("Loading behavioral rules...")
                        Spacer()
                    }
                } else if client.rules.isEmpty {
                    VStack(spacing: 12) {
                        Spacer()
                        Image(systemName: "doc.text")
                            .symairaText(.display)
                            .foregroundColor(.textMuted)
                        Text("No rules found")
                            .symairaText(.bodyEmphasized)
                            .foregroundColor(.textSecondary)
                        Text("Add rules via the `./symmemory rule add` terminal CLI.")
                            .symairaText(.callout)
                            .foregroundColor(.textMuted)
                        Spacer()
                    }
                } else {
                    ScrollView {
                        LazyVStack(spacing: 12) {
                            ForEach(client.rules) { rule in
                                RuleRow(rule: rule)
                            }
                        }
                        .padding()
                    }
                }
            }
        }
        .onAppear {
            Task { await client.fetchRules(scope: selectedScope) }
        }
    }
}

struct RuleRow: View {
    let rule: Rule
    @State private var isHovering = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(rule.scope.uppercased())
                    .symairaText(.caption)
.fontWeight(.bold)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(scopeColor.opacity(0.15))
                    .foregroundColor(scopeColor)
                    .cornerRadius(4)
                
                Spacer()
                
                Text(formattedDate)
                    .symairaText(.caption)
                    .foregroundColor(.textSecondary)
            }
            
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "quote.opening")
                    .foregroundColor(.goldPrimary.opacity(0.8))
                    .symairaText(.heading)
                    .shadow(color: .goldPrimary, radius: 2)
                
                Text(rule.content)
                    .symairaText(.body)
                    .foregroundColor(.textPrimary)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.top, 4)
        }
        .padding()
        .background(isHovering ? Color.cardBackgroundHover : Color.cardBackground)
        .cornerRadius(8)
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .stroke(isHovering ? Color.borderGlassHover : Color.borderGlass, lineWidth: 1)
        )
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.15)) {
                isHovering = hovering
            }
        }
    }
    
    private var scopeColor: Color {
        switch rule.scope.lowercased() {
        case "global": return .purple
        case "project": return .blue
        case "agent": return .green
        case "user": return .orange
        default: return .secondary
        }
    }
    
    private var formattedDate: String {
        let formatter = DateFormatter()
        formatter.dateStyle = .medium
        formatter.timeStyle = .short
        return formatter.string(from: rule.createdAt)
    }
}
