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
	export class LibrarySummaryDTO {
	    root: string;
	    total: number;
	    totalSize: number;
	    counts: KindCountsDTO;
	
	    static createFrom(source: any = {}) {
	        return new LibrarySummaryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.total = source["total"];
	        this.totalSize = source["totalSize"];
	        this.counts = this.convertValues(source["counts"], KindCountsDTO);
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

}

