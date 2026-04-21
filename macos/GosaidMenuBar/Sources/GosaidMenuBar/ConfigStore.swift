import Foundation

enum ConfigStore {
    static func load() throws -> Config {
        let url = Paths.configFile
        let fm = FileManager.default
        if !fm.fileExists(atPath: url.path) {
            let def = Config.makeDefault()
            try save(def)
            return def
        }
        let data = try Data(contentsOf: url)
        let dec = JSONDecoder()
        return try dec.decode(Config.self, from: data)
    }

    // Atomic write via tmp + rename, matching writeAtomic in
    // internal/config/store.go so the daemon never sees a half-written file.
    static func save(_ cfg: Config) throws {
        let url = Paths.configFile
        let fm = FileManager.default
        try fm.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )

        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try enc.encode(cfg)

        let tmp = url
            .deletingLastPathComponent()
            .appendingPathComponent("config-\(UUID().uuidString).json.tmp")
        try data.write(to: tmp, options: .atomic)

        // Replace existing file atomically.  FileManager.replaceItem gives us
        // cross-volume-safe atomic rename semantics.
        _ = try fm.replaceItemAt(url, withItemAt: tmp)
    }
}
