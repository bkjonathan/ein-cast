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

}

