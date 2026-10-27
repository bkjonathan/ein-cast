export namespace bridge {
	
	export class AppInfoDTO {
	    name: string;
	    version: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.platform = source["platform"];
	    }
	}
	export class FFmpegStatusDTO {
	    available: boolean;
	    path: string;
	    chosen: string;
	    installCommand: string;
	
	    static createFrom(source: any = {}) {
	        return new FFmpegStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.path = source["path"];
	        this.chosen = source["chosen"];
	        this.installCommand = source["installCommand"];
	    }
	}
	export class KindCountsDTO {
	    video: number;
	    audio: number;
	    image: number;
	
	    static createFrom(source: any = {}) {
	        return new KindCountsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.video = source["video"];
	        this.audio = source["audio"];
	        this.image = source["image"];
	    }
	}
	export class LibraryItemDTO {
	    id: string;
	    libraryId: string;
	    name: string;
	    relPath: string;
	    kind: string;
	    size: number;
	    modTime: number;
	    title: string;
	    year: number;
	    show: string;
	    season: number;
	    episode: number;
	    poster: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LibraryItemDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.libraryId = source["libraryId"];
	        this.name = source["name"];
	        this.relPath = source["relPath"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.title = source["title"];
	        this.year = source["year"];
	        this.show = source["show"];
	        this.season = source["season"];
	        this.episode = source["episode"];
	        this.poster = source["poster"];
	    }
	}
	export class LibraryStateDTO {
	    id: string;
	    name: string;
	    root: string;
	    total: number;
	    totalSize: number;
	    counts: KindCountsDTO;
	    scanning: boolean;
	    error: string;
	    watching: boolean;
	    hiddenFromTVs: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LibraryStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.root = source["root"];
	        this.total = source["total"];
	        this.totalSize = source["totalSize"];
	        this.counts = this.convertValues(source["counts"], KindCountsDTO);
	        this.scanning = source["scanning"];
	        this.error = source["error"];
	        this.watching = source["watching"];
	        this.hiddenFromTVs = source["hiddenFromTVs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogAttrDTO {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new LogAttrDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class LogLineDTO {
	    seq: number;
	    time: number;
	    level: string;
	    source: string;
	    message: string;
	    attrs: LogAttrDTO[];
	
	    static createFrom(source: any = {}) {
	        return new LogLineDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = source["time"];
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.attrs = this.convertValues(source["attrs"], LogAttrDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PortHolderDTO {
	    pid: number;
	    name: string;
	    self: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortHolderDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.name = source["name"];
	        this.self = source["self"];
	    }
	}
	export class ProfileDTO {
	    id: string;
	    name: string;
	    color: string;
	    allLibraries: boolean;
	    libraries: string[];
	    kinds: string[];
	    hiddenFolders: string[];
	    hasPin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.allLibraries = source["allLibraries"];
	        this.libraries = source["libraries"];
	        this.kinds = source["kinds"];
	        this.hiddenFolders = source["hiddenFolders"];
	        this.hasPin = source["hasPin"];
	    }
	}
	export class RemoteFieldDTO {
	    key: string;
	    label: string;
	    help: string;
	    placeholder: string;
	    kind: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RemoteFieldDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.help = source["help"];
	        this.placeholder = source["placeholder"];
	        this.kind = source["kind"];
	        this.required = source["required"];
	    }
	}
	export class RemoteFieldStateDTO {
	    key: string;
	    set: boolean;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteFieldStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.set = source["set"];
	        this.value = source["value"];
	    }
	}
	export class RemoteProviderDTO {
	    id: string;
	    public: boolean;
	    fields: RemoteFieldDTO[];
	
	    static createFrom(source: any = {}) {
	        return new RemoteProviderDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.public = source["public"];
	        this.fields = this.convertValues(source["fields"], RemoteFieldDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RemoteStatusDTO {
	    state: string;
	    provider: string;
	    public: boolean;
	    url: string;
	    error: string;
	    live: string;
	    pinSet: boolean;
	    sessions: number;
	    needsSetup: string[];
	
	    static createFrom(source: any = {}) {
	        return new RemoteStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.provider = source["provider"];
	        this.public = source["public"];
	        this.url = source["url"];
	        this.error = source["error"];
	        this.live = source["live"];
	        this.pinSet = source["pinSet"];
	        this.sessions = source["sessions"];
	        this.needsSetup = source["needsSetup"];
	    }
	}
	export class ServerStateDTO {
	    state: string;
	    name: string;
	    port: number;
	    localUrl: string;
	    lanUrls: string[];
	    nameUrls: string[];
	    discovery: string[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.name = source["name"];
	        this.port = source["port"];
	        this.localUrl = source["localUrl"];
	        this.lanUrls = source["lanUrls"];
	        this.nameUrls = source["nameUrls"];
	        this.discovery = source["discovery"];
	        this.error = source["error"];
	    }
	}
	export class SettingsDTO {
	    port: number;
	    name: string;
	    requireToken: boolean;
	    dlna: boolean;
	    autoStart: boolean;
	    keepAwake: boolean;
	    openAtLogin: boolean;
	    hideOnClose: boolean;
	    ffmpegPath: string;
	    accessToken: string;
	    tokenPersisted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.name = source["name"];
	        this.requireToken = source["requireToken"];
	        this.dlna = source["dlna"];
	        this.autoStart = source["autoStart"];
	        this.keepAwake = source["keepAwake"];
	        this.openAtLogin = source["openAtLogin"];
	        this.hideOnClose = source["hideOnClose"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.accessToken = source["accessToken"];
	        this.tokenPersisted = source["tokenPersisted"];
	    }
	}
	export class SettingsInputDTO {
	    port: number;
	    name: string;
	    requireToken: boolean;
	    dlna: boolean;
	    autoStart: boolean;
	    keepAwake: boolean;
	    openAtLogin: boolean;
	    hideOnClose: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInputDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.name = source["name"];
	        this.requireToken = source["requireToken"];
	        this.dlna = source["dlna"];
	        this.autoStart = source["autoStart"];
	        this.keepAwake = source["keepAwake"];
	        this.openAtLogin = source["openAtLogin"];
	        this.hideOnClose = source["hideOnClose"];
	    }
	}

}

