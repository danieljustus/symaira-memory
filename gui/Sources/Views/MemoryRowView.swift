import SwiftUI

struct MemoryRowView: View {
    let memory: Memory
    let onDelete: () -> Void
    
    @State private var isHovering = false
    @State private var isMetadataExpanded = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                // Scope Badge
                Text(memory.scope.uppercased())
                    .font(.system(size: 9, weight: .bold, design: .rounded))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .background(scopeColor.opacity(0.15))
                    .foregroundColor(scopeColor)
                    .cornerRadius(4)
                
                Spacer()
                
                // Timestamp
                Text(formattedDate)
                    .font(.caption)
                    .foregroundColor(.textSecondary)
                
                // Delete button (visible on hover)
                if isHovering {
                    Button(action: onDelete) {
                        Image(systemName: "trash")
                            .foregroundColor(.red)
                            .font(.caption)
                    }
                    .buttonStyle(.plain)
                    .transition(.opacity)
                    .padding(.leading, 8)
                }
            }
            
            // Content
            Text(memory.content)
                .font(.body)
                .textSelection(.enabled)
                .lineLimit(nil)
                .fixedSize(horizontal: false, vertical: true)
                .foregroundColor(.textPrimary)
            
            // Metadata section
            if let metadata = memory.metadata, !metadata.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Button {
                        withAnimation(.easeInOut(duration: 0.2)) {
                            isMetadataExpanded.toggle()
                        }
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: isMetadataExpanded ? "chevron.down" : "chevron.right")
                                .font(.system(size: 8))
                            Text("Metadata")
                                .font(.caption)
                                .fontWeight(.medium)
                        }
                        .foregroundColor(.textSecondary)
                    }
                    .buttonStyle(.plain)
                    
                    if isMetadataExpanded {
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(metadata.sorted(by: { $0.key < $1.key }), id: \.key) { key, val in
                                HStack(alignment: .top) {
                                    Text("\(key):")
                                        .font(.system(.caption, design: .monospaced))
                                        .fontWeight(.semibold)
                                        .foregroundColor(.textSecondary)
                                    Text(val)
                                        .font(.system(.caption, design: .monospaced))
                                        .foregroundColor(.textPrimary)
                                        .textSelection(.enabled)
                                }
                            }
                        }
                        .padding(.leading, 12)
                        .padding(.top, 2)
                    }
                }
            }
        }
        .padding()
        .background(isHovering ? Color.cardBackgroundHover : Color.cardBackground)
        .cornerRadius(10)
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(isHovering ? Color.borderGlassHover : Color.borderGlass, lineWidth: 1)
        )
        .onHover { hovering in
            withAnimation(.easeInOut(duration: 0.15)) {
                isHovering = hovering
            }
        }
    }
    
    private var scopeColor: Color {
        switch memory.scope.lowercased() {
        case "global": return .purple
        case "project": return .blue
        case "agent": return .green
        case "user": return .orange
        case "session": return .gray
        default: return .secondary
        }
    }
    
    private var formattedDate: String {
        let formatter = DateFormatter()
        formatter.dateStyle = .medium
        formatter.timeStyle = .short
        return formatter.string(from: memory.createdAt)
    }
}
