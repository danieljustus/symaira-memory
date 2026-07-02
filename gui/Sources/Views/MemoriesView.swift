import SwiftUI

struct MemoriesView: View {
    @EnvironmentObject var client: APIClient
    @State private var searchQuery = ""
    @State private var selectedScope = "all"
    @State private var isAddSheetPresented = false
    @State private var memoryToDelete: Memory? = nil
    @State private var searchTask: Task<Void, Never>? = nil
    
    let scopes = ["all", "global", "project", "agent", "user", "session"]
    
    @State private var isRefreshHovered = false
    @State private var isAddHovered = false
    
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
                // Toolbar header
                HStack {
                    HStack(spacing: 8) {
                        Image(systemName: "magnifyingglass")
                            .foregroundColor(.textSecondary)
                        TextField("Search memories semantically...", text: $searchQuery)
                            .textFieldStyle(.plain)
                            .foregroundColor(.textPrimary)
                            .autocorrectionDisabled()
                    }
                    .padding(8)
                    .background(Color.bgDarker)
                    .cornerRadius(8)
                    .overlay(
                        RoundedRectangle(cornerRadius: 8)
                            .stroke(Color.borderGlass, lineWidth: 1)
                    )
                    .frame(maxWidth: 350)
                    
                    Spacer()
                    
                    Button {
                        Task { await reloadData() }
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
                    .help("Refresh Memories")
                    
                    Button {
                        isAddSheetPresented = true
                    } label: {
                        HStack {
                            Image(systemName: "plus")
                            Text("Add Fact")
                                .fontWeight(.semibold)
                        }
                        .foregroundColor(.bgDarker)
                        .padding(.horizontal, 16)
                        .padding(.vertical, 8)
                        .background(Color.goldPrimary)
                        .cornerRadius(8)
                        .shadow(color: Color.goldPrimary.opacity(0.3), radius: 4, x: 0, y: 2)
                    }
                    .buttonStyle(.plain)
                    .scaleEffect(isAddHovered ? 1.02 : 1.0)
                    .onHover { hovering in
                        withAnimation(.easeInOut(duration: 0.15)) {
                            isAddHovered = hovering
                        }
                    }
                }
                .padding()
                .background(Color.bgDarker.opacity(0.3))
                
                Divider()
                    .background(Color.borderGlass)
                
                // Scope Picker Tab bar
                HStack {
                    ForEach(scopes, id: \.self) { scope in
                        Button {
                            selectedScope = scope
                            Task { await reloadData() }
                        } label: {
                            Text(scope.capitalized)
                                .font(.system(size: 13, weight: selectedScope == scope ? .semibold : .regular, design: .rounded))
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
                
                // List of Memories
                if client.isFetching && client.memories.isEmpty {
                    VStack {
                        Spacer()
                        ProgressView("Loading memories...")
                        Spacer()
                    }
                } else if client.memories.isEmpty {
                    VStack(spacing: 12) {
                        Spacer()
                        Image(systemName: "brain.head.profile")
                            .font(.system(size: 40))
                            .foregroundColor(.textMuted)
                        Text(searchQuery.isEmpty ? "No memories found" : "No semantic matches found")
                            .font(.headline)
                            .foregroundColor(.textSecondary)
                        Text(searchQuery.isEmpty ? "Create a fact using the 'Add Fact' button above." : "Try adjusting your search query.")
                            .font(.subheadline)
                            .foregroundColor(.textMuted)
                        Spacer()
                    }
                } else {
                    ScrollView {
                        LazyVStack(spacing: 12) {
                            ForEach(client.memories) { memory in
                                MemoryRowView(memory: memory) {
                                    memoryToDelete = memory
                                }
                            }
                        }
                        .padding()
                    }
                }
            }
        }
        .sheet(isPresented: $isAddSheetPresented) {
            AddMemoryView()
                .environmentObject(client)
        }
        .alert(item: $memoryToDelete) { memory in
            Alert(
                title: Text("Delete Memory?"),
                message: Text("Are you sure you want to permanently delete this fact: \"\(memory.content)\"?"),
                primaryButton: .destructive(Text("Delete")) {
                    Task {
                        _ = await client.deleteMemory(id: memory.id, currentScope: selectedScope)
                    }
                },
                secondaryButton: .cancel()
            )
        }
        .onAppear {
            Task { await reloadData() }
        }
        .onChange(of: searchQuery) { newValue in
            searchTask?.cancel()
            searchTask = Task {
                try? await Task.sleep(nanoseconds: 300_000_000)
                guard !Task.isCancelled else { return }
                
                if newValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    await client.fetchMemories(scope: selectedScope)
                } else {
                    await client.searchMemories(query: newValue, scope: selectedScope)
                }
            }
        }
    }
    
    private func reloadData() async {
        if searchQuery.isEmpty {
            await client.fetchMemories(scope: selectedScope)
        } else {
            await client.searchMemories(query: searchQuery, scope: selectedScope)
        }
    }
}
