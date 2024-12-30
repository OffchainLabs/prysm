load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep
load("@prysm//tools/go:def.bzl", "go_repository", "maybe")  # gazelle:keep

# Prysm's third party / external dependencies.
#
##################################################################
#
#                    ██████████████████
#                  ██                  ██
#                ██  ██████████████████  ██
#              ██  ██████████████████████  ██
#            ██  ██████████████████████████  ██
#          ██  ██████████████████████████████  ██
#        ██  ██████████████████████████████████  ██
#      ██  ██████████████████████████████████████  ██
#      ██  ██████    ██      ████  ████    ██████  ██
#      ██  ████  ████████  ████  ██  ██  ██  ████  ██
#      ██  ████  ████████  ████  ██  ██  ██  ████  ██
#      ██  ██████  ██████  ████  ██  ██    ██████  ██
#      ██  ████████  ████  ████  ██  ██  ████████  ██
#      ██  ████████  ████  ████  ██  ██  ████████  ██
#      ██  ████    ██████  ██████  ████  ████████  ██
#      ██  ██████████████████████████████████████  ██
#        ██  ██████████████████████████████████  ██
#          ██  ██████████████████████████████  ██
#            ██  ██████████████████████████  ██
#              ██  ██████████████████████  ██
#                ██  ██████████████████  ██
#                  ██                  ██
#                    ██████████████████
#
##################################################################
#           Make sure you have read DEPENDENCIES.md!
##################################################################
def prysm_deps():
    go_repository(
        name = "co_honnef_go_tools",
        importpath = "honnef.co/go/tools",
        sum = "h1:VUeHARd+9362HPYyFWjsRa6jBIAf2xWbDv6QXMRztbQ=",
        version = "v0.5.0-0.dev.0.20231205170804-aef76f4feee2",
    )
    go_repository(
        name = "com_github_aclements_go_moremath",
        importpath = "github.com/aclements/go-moremath",
        sum = "h1:xlwdaKcTNVW4PtpQb8aKA4Pjy0CdJHEqvFbAnvR5m2g=",
        version = "v0.0.0-20210112150236-f10218a38794",
    )
    go_repository(
        name = "com_github_afex_hystrix_go",
        importpath = "github.com/afex/hystrix-go",
        sum = "h1:rFw4nCn9iMW+Vajsk51NtYIcwSTkXr+JGrMd36kTDJw=",
        version = "v0.0.0-20180502004556-fa1af6a1f4f5",
    )
    go_repository(
        name = "com_github_alecthomas_kingpin_v2",
        importpath = "github.com/alecthomas/kingpin/v2",
        sum = "h1:f48lwail6p8zpO1bC4TxtqACaGqHYA22qkHjHpqDjYY=",
        version = "v2.4.0",
    )
    go_repository(
        name = "com_github_alecthomas_template",
        importpath = "github.com/alecthomas/template",
        sum = "h1:JYp7IbQjafoB+tBA3gMyHYHrpOtNuDiK/uB5uXxq5wM=",
        version = "v0.0.0-20190718012654-fb15b899a751",
    )
    go_repository(
        name = "com_github_alecthomas_units",
        importpath = "github.com/alecthomas/units",
        sum = "h1:s6gZFSlWYmbqAuRjVTiNNhvNRfY2Wxp9nhfyel4rklc=",
        version = "v0.0.0-20211218093645-b94a6e3cc137",
    )
    go_repository(
        name = "com_github_allegro_bigcache",
        importpath = "github.com/allegro/bigcache",
        sum = "h1:hg1sY1raCwic3Vnsvje6TT7/pnZba83LeFck5NrFKSc=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_andreasbriese_bbloom",
        importpath = "github.com/AndreasBriese/bbloom",
        sum = "h1:cTp8I5+VIoKjsnZuH8vjyaysT/ses3EvZeaV/1UkF2M=",
        version = "v0.0.0-20190825152654-46b345b51c96",
    )
    go_repository(
        name = "com_github_andybalholm_brotli",
        importpath = "github.com/andybalholm/brotli",
        sum = "h1:8uQZIdzKmjc/iuPu7O2ioW48L81FgatrcpfFmiq/cCs=",
        version = "v1.0.5",
    )
    go_repository(
        name = "com_github_anmitsu_go_shlex",
        importpath = "github.com/anmitsu/go-shlex",
        sum = "h1:kFOfPq6dUM1hTo4JG6LR5AXSUEsOjtdm0kw0FtQtMJA=",
        version = "v0.0.0-20161002113705-648efa622239",
    )
    go_repository(
        name = "com_github_apache_thrift",
        importpath = "github.com/apache/thrift",
        sum = "h1:5hryIiq9gtn+MiLVn0wP37kb/uTeRZgN08WoCsAhIhI=",
        version = "v0.13.0",
    )
    go_repository(
        name = "com_github_aristanetworks_fsnotify",
        importpath = "github.com/aristanetworks/fsnotify",
        sum = "h1:it2ydpY6k0aXB7qjb4vGhOYOL6YDC/sr8vhqwokFQwQ=",
        version = "v1.4.2",
    )
    go_repository(
        name = "com_github_aristanetworks_glog",
        importpath = "github.com/aristanetworks/glog",
        sum = "h1:Bmjk+DjIi3tTAU0wxGaFbfjGUqlxxSXARq9A96Kgoos=",
        version = "v0.0.0-20191112221043-67e8567f59f3",
    )
    go_repository(
        name = "com_github_aristanetworks_goarista",
        importpath = "github.com/aristanetworks/goarista",
        sum = "h1:XJH0YfVFKbq782tlNThzN/Ud5qm/cx6LXOA/P6RkTxc=",
        version = "v0.0.0-20200805130819-fd197cf57d96",
    )
    go_repository(
        name = "com_github_aristanetworks_splunk_hec_go",
        importpath = "github.com/aristanetworks/splunk-hec-go",
        sum = "h1:O7zlcm4ve7JvqTyEK3vSBh1LngLezraqcxv8Ya6tQFY=",
        version = "v0.3.3",
    )
    go_repository(
        name = "com_github_armon_circbuf",
        importpath = "github.com/armon/circbuf",
        sum = "h1:QEF07wC0T1rKkctt1RINW/+RMTVmiwxETico2l3gxJA=",
        version = "v0.0.0-20150827004946-bbbad097214e",
    )
    go_repository(
        name = "com_github_armon_go_metrics",
        importpath = "github.com/armon/go-metrics",
        sum = "h1:8GUt8eRujhVEGZFFEjBj46YV4rDjvGrNxb0KMWYkL2I=",
        version = "v0.0.0-20180917152333-f0300d1749da",
    )
    go_repository(
        name = "com_github_armon_go_radix",
        importpath = "github.com/armon/go-radix",
        sum = "h1:BUAU3CGlLvorLI26FmByPp2eC2qla6E1Tw+scpcg/to=",
        version = "v0.0.0-20180808171621-7fddfc383310",
    )
    go_repository(
        name = "com_github_armon_go_socks5",
        importpath = "github.com/armon/go-socks5",
        sum = "h1:0CwZNZbxp69SHPdPJAN/hZIm0C4OItdklCFmMRWYpio=",
        version = "v0.0.0-20160902184237-e75332964ef5",
    )
    go_repository(
        name = "com_github_aryann_difflib",
        importpath = "github.com/aryann/difflib",
        sum = "h1:pv34s756C4pEXnjgPfGYgdhg/ZdajGhyOvzx8k+23nw=",
        version = "v0.0.0-20170710044230-e206f873d14a",
    )
    go_repository(
        name = "com_github_aws_aws_lambda_go",
        importpath = "github.com/aws/aws-lambda-go",
        sum = "h1:SuCy7H3NLyp+1Mrfp+m80jcbi9KYWAs9/BXwppwRDzY=",
        version = "v1.13.3",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go",
        importpath = "github.com/aws/aws-sdk-go",
        sum = "h1:0xphMHGMLBrPMfxR2AmVjZKcMEESEgWF8Kru94BNByk=",
        version = "v1.27.0",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2",
        importpath = "github.com/aws/aws-sdk-go-v2",
        sum = "h1:+LXZ0sgo8quN9UOKXXzAWRT3FWd4NxeXWOZom9pE7GA=",
        version = "v1.21.2",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_config",
        importpath = "github.com/aws/aws-sdk-go-v2/config",
        sum = "h1:Aka9bI7n8ysuwPeFdm77nfbyHCAKQ3z9ghB3S/38zes=",
        version = "v1.18.45",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_credentials",
        importpath = "github.com/aws/aws-sdk-go-v2/credentials",
        sum = "h1:LU8vo40zBlo3R7bAvBVy/ku4nxGEyZe9N8MqAeFTzF8=",
        version = "v1.13.43",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_feature_ec2_imds",
        importpath = "github.com/aws/aws-sdk-go-v2/feature/ec2/imds",
        sum = "h1:PIktER+hwIG286DqXyvVENjgLTAwGgoeriLDD5C+YlQ=",
        version = "v1.13.13",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_internal_configsources",
        importpath = "github.com/aws/aws-sdk-go-v2/internal/configsources",
        sum = "h1:nFBQlGtkbPzp/NjZLuFxRqmT91rLJkgvsEQs68h962Y=",
        version = "v1.1.43",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_internal_endpoints_v2",
        importpath = "github.com/aws/aws-sdk-go-v2/internal/endpoints/v2",
        sum = "h1:JRVhO25+r3ar2mKGP7E0LDl8K9/G36gjlqca5iQbaqc=",
        version = "v2.4.37",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_internal_ini",
        importpath = "github.com/aws/aws-sdk-go-v2/internal/ini",
        sum = "h1:hze8YsjSh8Wl1rYa1CJpRmXP21BvOBuc76YhW0HsuQ4=",
        version = "v1.3.45",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_service_internal_presigned_url",
        importpath = "github.com/aws/aws-sdk-go-v2/service/internal/presigned-url",
        sum = "h1:WWZA/I2K4ptBS1kg0kV1JbBtG/umed0vwHRrmcr9z7k=",
        version = "v1.9.37",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_service_route53",
        importpath = "github.com/aws/aws-sdk-go-v2/service/route53",
        sum = "h1:/RPQNjh1sDIezpXaFIkZb7MlXnSyAqjVdAwcJuGYTqg=",
        version = "v1.30.2",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_service_sso",
        importpath = "github.com/aws/aws-sdk-go-v2/service/sso",
        sum = "h1:JuPGc7IkOP4AaqcZSIcyqLpFSqBWK32rM9+a1g6u73k=",
        version = "v1.15.2",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_service_ssooidc",
        importpath = "github.com/aws/aws-sdk-go-v2/service/ssooidc",
        sum = "h1:HFiiRkf1SdaAmV3/BHOFZ9DjFynPHj8G/UIO1lQS+fk=",
        version = "v1.17.3",
    )
    go_repository(
        name = "com_github_aws_aws_sdk_go_v2_service_sts",
        importpath = "github.com/aws/aws-sdk-go-v2/service/sts",
        sum = "h1:0BkLfgeDjfZnZ+MhB3ONb01u9pwFYTCZVhlsSSBvlbU=",
        version = "v1.23.2",
    )
    go_repository(
        name = "com_github_aws_smithy_go",
        importpath = "github.com/aws/smithy-go",
        sum = "h1:PS/durmlzvAFpQHDs4wi4sNNP9ExsqZh6IlfdHXgKK8=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_github_aymerick_douceur",
        importpath = "github.com/aymerick/douceur",
        sum = "h1:Mv+mAeH1Q+n9Fr+oyamOlAkUNPWPlA8PPGR0QAaYuPk=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_azure_azure_sdk_for_go_sdk_azcore",
        importpath = "github.com/Azure/azure-sdk-for-go/sdk/azcore",
        sum = "h1:8q4SaHjFsClSvuVne0ID/5Ka8u3fcIHyqkLjcFpNRHQ=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_github_azure_azure_sdk_for_go_sdk_internal",
        importpath = "github.com/Azure/azure-sdk-for-go/sdk/internal",
        sum = "h1:sXr+ck84g/ZlZUOZiNELInmMgOsuGwdjjVkEIde0OtY=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_azure_azure_sdk_for_go_sdk_storage_azblob",
        importpath = "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob",
        sum = "h1:gggzg0SUMs6SQbEw+3LoSsYf9YMjkupeAnHMX8O9mmY=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_bazelbuild_rules_go",
        importpath = "github.com/bazelbuild/rules_go",
        sum = "h1:Wxu7JjqnF78cKZbsBsARLSXx/jlGaSLCnUV3mTlyHvM=",
        version = "v0.23.2",
    )
    go_repository(
        name = "com_github_benbjohnson_clock",
        importpath = "github.com/benbjohnson/clock",
        sum = "h1:VvXlSJBzZpA/zum6Sj74hxwYI2DIxRWuNIoXAzHZz5o=",
        version = "v1.3.5",
    )
    go_repository(
        name = "com_github_beorn7_perks",
        importpath = "github.com/beorn7/perks",
        sum = "h1:VlbKKnNfV8bJzeqoa4cOKqO6bYr3WgKZxO8Z16+hsOM=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_bgentry_speakeasy",
        importpath = "github.com/bgentry/speakeasy",
        sum = "h1:ByYyxL9InA1OWqxJqqp2A5pYHUrCiAL6K3J+LKSsQkY=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_bits_and_blooms_bitset",
        importpath = "github.com/bits-and-blooms/bitset",
        sum = "h1:RMyy2mBBShArUAhfVRZJ2xyBO58KCBCtZFShw3umo6k=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_github_bradfitz_go_smtpd",
        importpath = "github.com/bradfitz/go-smtpd",
        sum = "h1:ckJgFhFWywOx+YLEMIJsTb+NV6NexWICk5+AMSuz3ss=",
        version = "v0.0.0-20170404230938-deb6d6237625",
    )
    go_repository(
        name = "com_github_bradfitz_gomemcache",
        importpath = "github.com/bradfitz/gomemcache",
        sum = "h1:7IjN4QP3c38xhg6wz8R3YjoU+6S9e7xBc0DAVLLIpHE=",
        version = "v0.0.0-20170208213004-1952afaa557d",
    )
    go_repository(
        name = "com_github_btcsuite_btcd_btcec_v2",
        importpath = "github.com/btcsuite/btcd/btcec/v2",
        sum = "h1:5n0X6hX0Zk+6omWcihdYvdAlGf2DfasC0GMf7DClJ3U=",
        version = "v2.3.2",
    )
    go_repository(
        name = "com_github_btcsuite_btcd_chaincfg_chainhash",
        importpath = "github.com/btcsuite/btcd/chaincfg/chainhash",
        sum = "h1:q0rUy8C/TYNBQS1+CGKw68tLOFYSNEs0TFnxxnS9+4U=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_buger_jsonparser",
        importpath = "github.com/buger/jsonparser",
        sum = "h1:D21IyuvjDCshj1/qq+pCNd3VZOAEI9jy6Bi131YlXgI=",
        version = "v0.0.0-20181115193947-bf1c66bbce23",
    )
    go_repository(
        name = "com_github_burntsushi_toml",
        importpath = "github.com/BurntSushi/toml",
        sum = "h1:o7IhLm0Msx3BaB+n3Ag7L8EVlByGnpq14C4YWiu/gL8=",
        version = "v1.3.2",
    )
    go_repository(
        name = "com_github_burntsushi_xgb",
        importpath = "github.com/BurntSushi/xgb",
        sum = "h1:1BDTz0u9nC3//pOCMdNH+CiXJVYJh5UQNCOBG7jbELc=",
        version = "v0.0.0-20160522181843-27f122750802",
    )
    go_repository(
        name = "com_github_casbin_casbin_v2",
        importpath = "github.com/casbin/casbin/v2",
        sum = "h1:bTwon/ECRx9dwBy2ewRVr5OiqjeXSGiTUY74sDPQi/g=",
        version = "v2.1.2",
    )
    go_repository(
        name = "com_github_cenkalti_backoff",
        importpath = "github.com/cenkalti/backoff",
        sum = "h1:tNowT99t7UNflLxfYYSlKYsBpXdEet03Pg2g16Swow4=",
        version = "v2.2.1+incompatible",
    )
    go_repository(
        name = "com_github_census_instrumentation_opencensus_proto",
        importpath = "github.com/census-instrumentation/opencensus-proto",
        sum = "h1:iKLQ0xPNFxR/2hzXZMrBo8f1j86j5WHzznCCQxV/b8g=",
        version = "v0.4.1",
    )
    go_repository(
        name = "com_github_cespare_cp",
        importpath = "github.com/cespare/cp",
        sum = "h1:nCb6ZLdB7NRaqsm91JtQTAme2SKJzXVsdPIPkyJr1MU=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_cespare_xxhash",
        importpath = "github.com/cespare/xxhash",
        sum = "h1:a6HrQnmkObjyL+Gs60czilIUGqrzKutQD6XZog3p+ko=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_cespare_xxhash_v2",
        importpath = "github.com/cespare/xxhash/v2",
        sum = "h1:UL815xU9SqsFlibzuggzjXhog7bL6oX9BbNZnL2UFvs=",
        version = "v2.3.0",
    )
    go_repository(
        name = "com_github_chzyer_logex",
        importpath = "github.com/chzyer/logex",
        sum = "h1:XHDu3E6q+gdHgsdTPH6ImJMIp436vR6MPtH8gP05QzM=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_chzyer_readline",
        importpath = "github.com/chzyer/readline",
        sum = "h1:upd/6fQk4src78LMRzh5vItIt361/o4uq553V8B5sGI=",
        version = "v1.5.1",
    )
    go_repository(
        name = "com_github_chzyer_test",
        importpath = "github.com/chzyer/test",
        sum = "h1:p3BQDXSxOhOG0P9z6/hGnII4LGiEPOYBhs8asl/fC04=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_cilium_ebpf",
        importpath = "github.com/cilium/ebpf",
        sum = "h1:64sn2K3UKw8NbP/blsixRpF3nXuyhz/VjRlRzvlBRu4=",
        version = "v0.9.1",
    )
    go_repository(
        name = "com_github_clbanning_x2j",
        importpath = "github.com/clbanning/x2j",
        sum = "h1:EdRZT3IeKQmfCSrgo8SZ8V3MEnskuJP0wCYNpe+aiXo=",
        version = "v0.0.0-20191024224557-825249438eec",
    )
    go_repository(
        name = "com_github_client9_misspell",
        importpath = "github.com/client9/misspell",
        sum = "h1:ta993UF76GwbvJcIo3Y68y/M3WxlpEHPWIGDkJYwzJI=",
        version = "v0.3.4",
    )
    go_repository(
        name = "com_github_cloudflare_cloudflare_go",
        importpath = "github.com/cloudflare/cloudflare-go",
        sum = "h1:ErwCYDjFCYppDJlDJ/5WhsSmzegAUe2+K9qgFyQDg3M=",
        version = "v0.79.0",
    )
    go_repository(
        name = "com_github_cloudykit_fastprinter",
        importpath = "github.com/CloudyKit/fastprinter",
        sum = "h1:sR+/8Yb4slttB4vD+b9btVEnWgL3Q00OBTzVT8B9C0c=",
        version = "v0.0.0-20200109182630-33d98a066a53",
    )
    go_repository(
        name = "com_github_cloudykit_jet_v6",
        importpath = "github.com/CloudyKit/jet/v6",
        sum = "h1:EpcZ6SR9n28BUGtNJSvlBqf90IpjeFr36Tizxhn/oME=",
        version = "v6.2.0",
    )
    go_repository(
        name = "com_github_cncf_udpa_go",
        importpath = "github.com/cncf/udpa/go",
        sum = "h1:cqQfy1jclcSy/FwLjemeg3SR1yaINm74aQyupQ0Bl8M=",
        version = "v0.0.0-20201120205902-5459f2c99403",
    )
    go_repository(
        name = "com_github_cncf_xds_go",
        importpath = "github.com/cncf/xds/go",
        sum = "h1:ga8SEFjZ60pxLcmhnThWgvH2wg8376yUJmPhEH4H3kw=",
        version = "v0.0.0-20240423153145-555b57ec207b",
    )
    go_repository(
        name = "com_github_cockroachdb_datadriven",
        importpath = "github.com/cockroachdb/datadriven",
        sum = "h1:otljaYPt5hWxV3MUfO5dFPFiOXg9CyG5/kCfayTqsJ4=",
        version = "v1.0.3-0.20230413201302-be42291fc80f",
    )
    go_repository(
        name = "com_github_cockroachdb_errors",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/cockroachdb/errors",
        sum = "h1:xSEW75zKaKCWzR3OfxXUxgrk/NtT4G1MiOv5lWZazG8=",
        version = "v1.11.1",
    )
    go_repository(
        name = "com_github_cockroachdb_logtags",
        importpath = "github.com/cockroachdb/logtags",
        sum = "h1:r6VH0faHjZeQy818SGhaone5OnYfxFR/+AzdY3sf5aE=",
        version = "v0.0.0-20230118201751-21c54148d20b",
    )
    go_repository(
        name = "com_github_cockroachdb_pebble",
        importpath = "github.com/cockroachdb/pebble",
        sum = "h1:aPEJyR4rPBvDmeyi+l/FS/VtA00IWvjeFvjen1m1l1A=",
        version = "v0.0.0-20230928194634-aa077af62593",
    )
    go_repository(
        name = "com_github_cockroachdb_redact",
        importpath = "github.com/cockroachdb/redact",
        sum = "h1:u1PMllDkdFfPWaNGMyLD1+so+aq3uUItthCFqzwPJ30=",
        version = "v1.1.5",
    )
    go_repository(
        name = "com_github_cockroachdb_sentry_go",
        importpath = "github.com/cockroachdb/sentry-go",
        sum = "h1:IKgmqgMQlVJIZj19CdocBeSfSaiCbEBZGKODaixqtHM=",
        version = "v0.6.1-cockroachdb.2",
    )
    go_repository(
        name = "com_github_cockroachdb_tokenbucket",
        importpath = "github.com/cockroachdb/tokenbucket",
        sum = "h1:zuQyyAKVxetITBuuhv3BI9cMrmStnpT18zmgmTxunpo=",
        version = "v0.0.0-20230807174530-cc333fc44b06",
    )
    go_repository(
        name = "com_github_codahale_hdrhistogram",
        importpath = "github.com/codahale/hdrhistogram",
        sum = "h1:qMd81Ts1T2OTKmB4acZcyKaMtRnY5Y44NuXGX2GFJ1w=",
        version = "v0.0.0-20161010025455-3a0bb77429bd",
    )
    go_repository(
        name = "com_github_codegangsta_inject",
        importpath = "github.com/codegangsta/inject",
        sum = "h1:sDMmm+q/3+BukdIpxwO365v/Rbspp2Nt5XntgQRXq8Q=",
        version = "v0.0.0-20150114235600-33e0aa1cb7c0",
    )
    go_repository(
        name = "com_github_consensys_bavard",
        importpath = "github.com/consensys/bavard",
        sum = "h1:oLhMLOFGTLdlda/kma4VOJazblc7IM5y5QPd2A/YjhQ=",
        version = "v0.1.13",
    )
    go_repository(
        name = "com_github_consensys_gnark_crypto",
        importpath = "github.com/consensys/gnark-crypto",
        sum = "h1:lHH39WuuFgVHONRl3J0LRBtuYdQTumFSDtJF7HpyG8M=",
        version = "v0.12.1",
    )
    go_repository(
        name = "com_github_containerd_cgroups",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/containerd/cgroups",
        sum = "h1:v8rEWFl6EoqHB+swVNjVoCJE8o3jX7e8nqBGPLaDFBM=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_coreos_go_semver",
        importpath = "github.com/coreos/go-semver",
        sum = "h1:3Jm3tLmsgAYcjC+4Up7hJrFBPr+n7rAqYeSw/SZazuY=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_coreos_go_systemd",
        importpath = "github.com/coreos/go-systemd",
        sum = "h1:iW4rZ826su+pqaw19uhpSCzhj44qo35pNgKFGqzDKkU=",
        version = "v0.0.0-20191104093116-d3cd4ed1dbcf",
    )
    go_repository(
        name = "com_github_coreos_go_systemd_v22",
        importpath = "github.com/coreos/go-systemd/v22",
        sum = "h1:RrqgGjYQKalulkV8NGVIfkXQf6YYmOyiJKk8iXXhfZs=",
        version = "v22.5.0",
    )
    go_repository(
        name = "com_github_coreos_pkg",
        importpath = "github.com/coreos/pkg",
        sum = "h1:CAKfRE2YtTUIjjh1bkBtyYFaUT/WmOqsJjgtihT0vMI=",
        version = "v0.0.0-20160727233714-3ac0863d7acf",
    )
    go_repository(
        name = "com_github_cpuguy83_go_md2man_v2",
        importpath = "github.com/cpuguy83/go-md2man/v2",
        sum = "h1:qMCsGGgs+MAzDFyp9LpAe1Lqy/fY/qCovCm0qnXZOBM=",
        version = "v2.0.3",
    )
    go_repository(
        name = "com_github_crate_crypto_go_ipa",
        importpath = "github.com/crate-crypto/go-ipa",
        sum = "h1:DuBDHVjgGMPki7bAyh91+3cF1Vh34sAEdH8JQgbc2R0=",
        version = "v0.0.0-20230601170251-1830d0757c80",
    )
    go_repository(
        name = "com_github_crate_crypto_go_kzg_4844",
        importpath = "github.com/crate-crypto/go-kzg-4844",
        sum = "h1:C0vgZRk4q4EZ/JgPfzuSoxdCq3C3mOZMBShovmncxvA=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_github_creack_pty",
        importpath = "github.com/creack/pty",
        sum = "h1:uDmaGzcdjhF4i/plgjmEsriH11Y0o7RKapEf/LDaM3w=",
        version = "v1.1.9",
    )
    go_repository(
        name = "com_github_cyberdelia_templates",
        importpath = "github.com/cyberdelia/templates",
        sum = "h1:/ovYnF02fwL0kvspmy9AuyKg1JhdTRUgPw4nUxd9oZM=",
        version = "v0.0.0-20141128023046-ca7fffd4298c",
    )
    go_repository(
        name = "com_github_d4l3k_messagediff",
        importpath = "github.com/d4l3k/messagediff",
        sum = "h1:ZcAIMYsUg0EAp9X+tt8/enBE/Q8Yd5kzPynLyKptt9U=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_datadog_zstd",
        importpath = "github.com/DataDog/zstd",
        sum = "h1:oWf5W7GtOLgp6bciQYDmhHHjdhYkALu6S/5Ni9ZgSvQ=",
        version = "v1.5.5",
    )
    go_repository(
        name = "com_github_davecgh_go_spew",
        importpath = "github.com/davecgh/go-spew",
        sum = "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_davidlazar_go_crypto",
        importpath = "github.com/davidlazar/go-crypto",
        sum = "h1:pFUpOrbxDR6AkioZ1ySsx5yxlDQZ8stG2b88gTPxgJU=",
        version = "v0.0.0-20200604182044-b73af7476f6c",
    )
    go_repository(
        name = "com_github_deckarep_golang_set_v2",
        importpath = "github.com/deckarep/golang-set/v2",
        sum = "h1:hn6cEZtQ0h3J8kFrHR/NrzyOoTnjgW1+FmNJzQ7y/sA=",
        version = "v2.5.0",
    )
    go_repository(
        name = "com_github_decred_dcrd_crypto_blake256",
        importpath = "github.com/decred/dcrd/crypto/blake256",
        sum = "h1:7PltbUIQB7u/FfZ39+DGa/ShuMyJ5ilcvdfma9wOH6Y=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_decred_dcrd_dcrec_secp256k1_v4",
        importpath = "github.com/decred/dcrd/dcrec/secp256k1/v4",
        sum = "h1:rpfIENRNNilwHwZeG5+P150SMrnNEcHYvcCuK6dPZSg=",
        version = "v4.3.0",
    )
    go_repository(
        name = "com_github_deepmap_oapi_codegen",
        importpath = "github.com/deepmap/oapi-codegen",
        sum = "h1:SegyeYGcdi0jLLrpbCMoJxnUUn8GBXHsvr4rbzjuhfU=",
        version = "v1.8.2",
    )
    go_repository(
        name = "com_github_dgraph_io_badger",
        importpath = "github.com/dgraph-io/badger",
        sum = "h1:mNw0qs90GVgGGWylh0umH5iag1j6n/PeJtNvL6KY/x8=",
        version = "v1.6.2",
    )
    go_repository(
        name = "com_github_dgraph_io_ristretto",
        importpath = "github.com/dgraph-io/ristretto",
        sum = "h1:cNcG4c2n5xanQzp2hMyxDxPYVQmZ91y4WN6fJFlndLo=",
        version = "v0.0.4-0.20210318174700-74754f61e018",
    )
    go_repository(
        name = "com_github_dgrijalva_jwt_go",
        importpath = "github.com/dgrijalva/jwt-go",
        sum = "h1:7qlOGliEKZXTDg6OTjfoBKDXWrumCAMpl/TFQ4/5kLM=",
        version = "v3.2.0+incompatible",
    )
    go_repository(
        name = "com_github_dgryski_go_farm",
        importpath = "github.com/dgryski/go-farm",
        sum = "h1:tdlZCpZ/P9DhczCTSixgIKmwPv6+wP5DGjqLYw5SUiA=",
        version = "v0.0.0-20190423205320-6a90982ecee2",
    )
    go_repository(
        name = "com_github_dlclark_regexp2",
        importpath = "github.com/dlclark/regexp2",
        sum = "h1:7lJfhqlPssTb1WQx4yvTHN0uElPEv52sbaECrAQxjAo=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_github_docker_go_units",
        importpath = "github.com/docker/go-units",
        sum = "h1:69rxXcBk27SvSaaxTtLh/8llcHD8vYHT7WSdRZ/jvr4=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_github_dop251_goja",
        importpath = "github.com/dop251/goja",
        sum = "h1:qwcF+vdFrvPSEUDSX5RVoRccG8a5DhOdWdQ4zN62zzo=",
        version = "v0.0.0-20230806174421-c933cf95e127",
    )
    go_repository(
        name = "com_github_dop251_goja_nodejs",
        importpath = "github.com/dop251/goja_nodejs",
        sum = "h1:W1n4DvpzZGOISgp7wWNtraLcHtnmnTwBlJidqtMIuwQ=",
        version = "v0.0.0-20211022123610-8dd9abb0616d",
    )
    go_repository(
        name = "com_github_dustin_go_humanize",
        importpath = "github.com/dustin/go-humanize",
        sum = "h1:VSnTsYCnlFHaM2/igO1h6X3HA71jcobQuxemgkq4zYo=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_eapache_go_resiliency",
        importpath = "github.com/eapache/go-resiliency",
        sum = "h1:v7g92e/KSN71Rq7vSThKaWIq68fL4YHvWyiUKorFR1Q=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_eapache_go_xerial_snappy",
        importpath = "github.com/eapache/go-xerial-snappy",
        sum = "h1:YEetp8/yCZMuEPMUDHG0CW/brkkEp8mzqk2+ODEitlw=",
        version = "v0.0.0-20180814174437-776d5712da21",
    )
    go_repository(
        name = "com_github_eapache_queue",
        importpath = "github.com/eapache/queue",
        sum = "h1:YOEu7KNc61ntiQlcEeUIoDTJ2o8mQznoNvUhiigpIqc=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_edsrzf_mmap_go",
        importpath = "github.com/edsrzf/mmap-go",
        sum = "h1:CEBF7HpRnUCSJgGUb5h1Gm7e3VkmVDrR8lvWVLtrOFw=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_eknkc_amber",
        importpath = "github.com/eknkc/amber",
        sum = "h1:clC1lXBpe2kTj2VHdaIu9ajZQe4kcEY9j0NsnDDBZ3o=",
        version = "v0.0.0-20171010120322-cdade1c07385",
    )
    go_repository(
        name = "com_github_elastic_gosigar",
        importpath = "github.com/elastic/gosigar",
        sum = "h1:xwkKwPia+hSfg9GqrCUKYdId102m9qTJIIr7egmK/uo=",
        version = "v0.14.3",
    )
    go_repository(
        name = "com_github_emicklei_dot",
        importpath = "github.com/emicklei/dot",
        sum = "h1:Ase39UD9T9fRBOb5ptgpixrxfx8abVzNWZi2+lr53PI=",
        version = "v0.11.0",
    )
    go_repository(
        name = "com_github_emicklei_go_restful_v3",
        importpath = "github.com/emicklei/go-restful/v3",
        sum = "h1:rAQeMHw1c7zTmncogyy8VvRZwtkmkZ4FxERmMY4rD+g=",
        version = "v3.11.0",
    )
    go_repository(
        name = "com_github_envoyproxy_go_control_plane",
        importpath = "github.com/envoyproxy/go-control-plane",
        sum = "h1:4X+VP1GHd1Mhj6IB5mMeGbLCleqxjletLK6K0rbxyZI=",
        version = "v0.12.0",
    )
    go_repository(
        name = "com_github_envoyproxy_protoc_gen_validate",
        importpath = "github.com/envoyproxy/protoc-gen-validate",
        sum = "h1:gVPz/FMfvh57HdSJQyvBtF00j8JU4zdyUgIUNhlgg0A=",
        version = "v1.0.4",
    )
    go_repository(
        name = "com_github_ethereum_c_kzg_4844",
        build_directives = [
            "gazelle:resolve go github.com/supranational/blst/bindings/go @com_github_supranational_blst//:go_default_library",
        ],
        importpath = "github.com/ethereum/c-kzg-4844",
        patch_args = ["-p1"],
        patches = ["//third_party:com_github_ethereum_c_kzg_4844.patch"],
        sum = "h1:3MS1s4JtA868KpJxroZoepdV0ZKBp3u/O5HcZ7R3nlY=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_ethereum_go_ethereum",
        build_directives = [
            "gazelle:resolve go github.com/karalabe/usb @prysm//third_party/usb:go_default_library",
        ],
        importpath = "github.com/ethereum/go-ethereum",
        patch_args = ["-p1"],
        patches = [
            "//third_party:com_github_ethereum_go_ethereum_secp256k1.patch",
        ],
        sum = "h1:U6TCRciCqZRe4FPXmy1sMGxTfuk8P7u2UoinF3VbaFk=",
        version = "v1.13.5",
    )
    go_repository(
        name = "com_github_evanphx_json_patch",
        importpath = "github.com/evanphx/json-patch",
        sum = "h1:4onqiflcdA9EOZ4RxV643DvftH5pOlLGNtQ5lPWQu84=",
        version = "v4.12.0+incompatible",
    )
    go_repository(
        name = "com_github_fatih_color",
        importpath = "github.com/fatih/color",
        sum = "h1:8LOYc1KYPPmyKMuN8QV2DNRWNbLo6LZ0iLs8+mlH53w=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_github_fatih_structs",
        importpath = "github.com/fatih/structs",
        sum = "h1:Q7juDM0QtcnhCpeyLGQKyg4TOIghuNXrkL32pHAUMxo=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_felixge_fgprof",
        importpath = "github.com/felixge/fgprof",
        sum = "h1:VvyZxILNuCiUCSXtPtYmmtGvb65nqXh2QFWc0Wpf2/g=",
        version = "v0.9.3",
    )
    go_repository(
        name = "com_github_ferranbt_fastssz",
        importpath = "github.com/ferranbt/fastssz",
        sum = "h1:9VDpsWq096+oGMDTT/SgBD/VgZYf4pTF+KTPmZ+OaKM=",
        version = "v0.0.0-20210120143747-11b9eff30ea9",
    )
    go_repository(
        name = "com_github_fjl_gencodec",
        importpath = "github.com/fjl/gencodec",
        sum = "h1:bBLctRc7kr01YGvaDfgLbTwjFNW5jdp5y5rj8XXBHfY=",
        version = "v0.0.0-20230517082657-f9840df7b83e",
    )
    go_repository(
        name = "com_github_fjl_memsize",
        importpath = "github.com/fjl/memsize",
        sum = "h1:FtmdgXiUlNeRsoNMFlKLDt+S+6hbjVMEW6RGQ7aUf7c=",
        version = "v0.0.0-20190710130421-bcb5799ab5e5",
    )
    go_repository(
        name = "com_github_flosch_pongo2_v4",
        importpath = "github.com/flosch/pongo2/v4",
        sum = "h1:gv+5Pe3vaSVmiJvh/BZa82b7/00YUGm0PIyVVLop0Hw=",
        version = "v4.0.2",
    )
    go_repository(
        name = "com_github_flynn_go_shlex",
        importpath = "github.com/flynn/go-shlex",
        sum = "h1:BHsljHzVlRcyQhjrss6TZTdY2VfCqZPbv5k3iBFa2ZQ=",
        version = "v0.0.0-20150515145356-3f9db97f8568",
    )
    go_repository(
        name = "com_github_flynn_noise",
        importpath = "github.com/flynn/noise",
        sum = "h1:KjPQoQCEFdZDiP03phOvGi11+SVVhBG2wOWAorLsstg=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_fortytw2_leaktest",
        importpath = "github.com/fortytw2/leaktest",
        sum = "h1:u8491cBMTQ8ft8aeV+adlcytMZylmA5nnwwkRZjI8vw=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_francoispqt_gojay",
        importpath = "github.com/francoispqt/gojay",
        sum = "h1:d2m3sFjloqoIUQU3TsHBgj6qg/BVGlTBeHDUmyJnXKk=",
        version = "v1.2.13",
    )
    go_repository(
        name = "com_github_franela_goblin",
        importpath = "github.com/franela/goblin",
        sum = "h1:gb2Z18BhTPJPpLQWj4T+rfKHYCHxRHCtRxhKKjRidVw=",
        version = "v0.0.0-20200105215937-c9ffbefa60db",
    )
    go_repository(
        name = "com_github_franela_goreq",
        importpath = "github.com/franela/goreq",
        sum = "h1:a9ENSRDFBUPkJ5lCgVZh26+ZbGyoVJG7yb5SSzF5H54=",
        version = "v0.0.0-20171204163338-bcd34c9993f8",
    )
    go_repository(
        name = "com_github_frankban_quicktest",
        importpath = "github.com/frankban/quicktest",
        sum = "h1:2QxQoC1TS09S7fhCPsrvqYdvP1H5M1P1ih5ABm3BTYk=",
        version = "v1.7.2",
    )
    go_repository(
        name = "com_github_fsnotify_fsnotify",
        importpath = "github.com/fsnotify/fsnotify",
        sum = "h1:n+5WquG0fcWoWp6xPWfHdbskMCQaFnG6PfBrh1Ky4HY=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_github_fxamacker_cbor_v2",
        importpath = "github.com/fxamacker/cbor/v2",
        sum = "h1:sU6J2usfADwWlYDAFhZBQ6TnLFBHxgesMrQfQgk1tWA=",
        version = "v2.6.0",
    )
    go_repository(
        name = "com_github_garslo_gogen",
        importpath = "github.com/garslo/gogen",
        sum = "h1:IZqZOB2fydHte3kUgxrzK5E1fW7RQGeDwE8F/ZZnUYc=",
        version = "v0.0.0-20170306192744-1d203ffc1f61",
    )
    go_repository(
        name = "com_github_garyburd_redigo",
        importpath = "github.com/garyburd/redigo",
        sum = "h1:0VruCpn7yAIIu7pWVClQC8wxCJEcG3nyzpMSHKi1PQc=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_github_gballet_go_libpcsclite",
        importpath = "github.com/gballet/go-libpcsclite",
        sum = "h1:f6D9Hr8xV8uYKlyuj8XIruxlh9WjVjdh1gIicAS7ays=",
        version = "v0.0.0-20191108122812-4678299bea08",
    )
    go_repository(
        name = "com_github_gballet_go_verkle",
        importpath = "github.com/gballet/go-verkle",
        sum = "h1:vMT47RYsrftsHSTQhqXwC3BYflo38OLC3Y4LtXtLyU0=",
        version = "v0.0.0-20230607174250-df487255f46b",
    )
    go_repository(
        name = "com_github_gdamore_encoding",
        importpath = "github.com/gdamore/encoding",
        sum = "h1:+7OoQ1Bc6eTm5niUzBa0Ctsh6JbMW6Ra+YNuAtDBdko=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_gdamore_tcell_v2",
        importpath = "github.com/gdamore/tcell/v2",
        sum = "h1:OKbluoP9VYmJwZwq/iLb4BxwKcwGthaa1YNBJIyCySg=",
        version = "v2.6.0",
    )
    go_repository(
        name = "com_github_getkin_kin_openapi",
        importpath = "github.com/getkin/kin-openapi",
        sum = "h1:6awGqF5nG5zkVpMsAih1QH4VgzS8phTxECUWIFo7zko=",
        version = "v0.61.0",
    )
    go_repository(
        name = "com_github_getsentry_sentry_go",
        importpath = "github.com/getsentry/sentry-go",
        sum = "h1:q6Eo+hS+yoJlTO3uu/azhQadsD8V+jQn2D8VvX1eOyI=",
        version = "v0.25.0",
    )
    go_repository(
        name = "com_github_ghemawat_stream",
        importpath = "github.com/ghemawat/stream",
        sum = "h1:r5GgOLGbza2wVHRzK7aAj6lWZjfbAwiu/RDCVOKjRyM=",
        version = "v0.0.0-20171120220530-696b145b53b9",
    )
    go_repository(
        name = "com_github_ghodss_yaml",
        importpath = "github.com/ghodss/yaml",
        sum = "h1:wQHKEahhL6wmXdzwWG11gIVCkOv05bNOh+Rxn0yngAk=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_gin_contrib_sse",
        importpath = "github.com/gin-contrib/sse",
        sum = "h1:Y/yl/+YNO8GZSjAhjMsSuLt29uWRFHdHYUb5lYOV9qE=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_gin_gonic_gin",
        importpath = "github.com/gin-gonic/gin",
        sum = "h1:4+fr/el88TOO3ewCmQr8cx/CtZ/umlIRIs5M4NTNjf8=",
        version = "v1.8.1",
    )
    go_repository(
        name = "com_github_gliderlabs_ssh",
        importpath = "github.com/gliderlabs/ssh",
        sum = "h1:j3L6gSLQalDETeEg/Jg0mGY0/y/N6zI2xX1978P0Uqw=",
        version = "v0.1.1",
    )
    go_repository(
        name = "com_github_go_chi_chi_v5",
        importpath = "github.com/go-chi/chi/v5",
        sum = "h1:DBPx88FjZJH3FsICfDAfIfnb7XxKIYVGG6lOPlhENAg=",
        version = "v5.0.0",
    )
    go_repository(
        name = "com_github_go_errors_errors",
        importpath = "github.com/go-errors/errors",
        sum = "h1:J6MZopCL4uSllY1OfXM374weqZFFItUbrImctkmUxIA=",
        version = "v1.4.2",
    )
    go_repository(
        name = "com_github_go_gl_glfw",
        importpath = "github.com/go-gl/glfw",
        sum = "h1:QbL/5oDUmRBzO9/Z7Seo6zf912W/a6Sr4Eu0G/3Jho0=",
        version = "v0.0.0-20190409004039-e6da0acd62b1",
    )
    go_repository(
        name = "com_github_go_gl_glfw_v3_3_glfw",
        importpath = "github.com/go-gl/glfw/v3.3/glfw",
        sum = "h1:WtGNWLvXpe6ZudgnXrq0barxBImvnnJoMEhXAzcbM0I=",
        version = "v0.0.0-20200222043503-6f7a984d4dc4",
    )
    go_repository(
        name = "com_github_go_kit_kit",
        importpath = "github.com/go-kit/kit",
        sum = "h1:dXFJfIHVvUcpSgDOV+Ne6t7jXri8Tfv2uOLHUZ2XNuo=",
        version = "v0.10.0",
    )
    go_repository(
        name = "com_github_go_kit_log",
        importpath = "github.com/go-kit/log",
        sum = "h1:MRVx0/zhvdseW+Gza6N9rVzU/IVzaeE1SFI4raAhmBU=",
        version = "v0.2.1",
    )
    go_repository(
        name = "com_github_go_logfmt_logfmt",
        importpath = "github.com/go-logfmt/logfmt",
        sum = "h1:otpy5pqBCBZ1ng9RQ0dPu4PN7ba75Y/aA+UpowDyNVA=",
        version = "v0.5.1",
    )
    go_repository(
        name = "com_github_go_logr_logr",
        importpath = "github.com/go-logr/logr",
        sum = "h1:6pFjapn8bFcIbiKo3XT4j/BhANplGihG6tvd+8rYgrY=",
        version = "v1.4.2",
    )
    go_repository(
        name = "com_github_go_logr_stdr",
        importpath = "github.com/go-logr/stdr",
        sum = "h1:hSWxHoqTgW2S2qGc0LTAI563KZ5YKYRhT3MFKZMbjag=",
        version = "v1.2.2",
    )
    go_repository(
        name = "com_github_go_martini_martini",
        importpath = "github.com/go-martini/martini",
        sum = "h1:xveKWz2iaueeTaUgdetzel+U7exyigDYBryyVfV/rZk=",
        version = "v0.0.0-20170121215854-22fa46961aab",
    )
    go_repository(
        name = "com_github_go_ole_go_ole",
        importpath = "github.com/go-ole/go-ole",
        sum = "h1:Dt6ye7+vXGIKZ7Xtk4s6/xVdGDQynvom7xCFEdWr6uE=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_go_openapi_jsonpointer",
        importpath = "github.com/go-openapi/jsonpointer",
        sum = "h1:eCs3fxoIi3Wh6vtgmLTOjdhSpiqphQ+DaPn38N2ZdrE=",
        version = "v0.19.6",
    )
    go_repository(
        name = "com_github_go_openapi_jsonreference",
        importpath = "github.com/go-openapi/jsonreference",
        sum = "h1:3sVjiK66+uXK/6oQ8xgcRKcFgQ5KXa2KvnJRumpMGbE=",
        version = "v0.20.2",
    )
    go_repository(
        name = "com_github_go_openapi_swag",
        importpath = "github.com/go-openapi/swag",
        sum = "h1:yMBqmnQ0gyZvEb/+KzuWZOXgllrXT4SADYbvDaXHv/g=",
        version = "v0.22.3",
    )
    go_repository(
        name = "com_github_go_playground_assert_v2",
        importpath = "github.com/go-playground/assert/v2",
        sum = "h1:JvknZsQTYeFEAhQwI4qEt9cyV5ONwRHC+lYKSsYSR8s=",
        version = "v2.2.0",
    )
    go_repository(
        name = "com_github_go_playground_locales",
        importpath = "github.com/go-playground/locales",
        sum = "h1:EWaQ/wswjilfKLTECiXz7Rh+3BjFhfDFKv/oXslEjJA=",
        version = "v0.14.1",
    )
    go_repository(
        name = "com_github_go_playground_universal_translator",
        importpath = "github.com/go-playground/universal-translator",
        sum = "h1:Bcnm0ZwsGyWbCzImXv+pAJnYK9S473LQFuzCbDbfSFY=",
        version = "v0.18.1",
    )
    go_repository(
        name = "com_github_go_playground_validator_v10",
        importpath = "github.com/go-playground/validator/v10",
        sum = "h1:cFRQdfaSMCOSfGCCLB20MHvuoHb/s5G8L5pu2ppK5AQ=",
        version = "v10.13.0",
    )
    go_repository(
        name = "com_github_go_sourcemap_sourcemap",
        importpath = "github.com/go-sourcemap/sourcemap",
        sum = "h1:W1iEw64niKVGogNgBN3ePyLFfuisuzeidWPMPWmECqU=",
        version = "v2.1.3+incompatible",
    )
    go_repository(
        name = "com_github_go_sql_driver_mysql",
        importpath = "github.com/go-sql-driver/mysql",
        sum = "h1:7LxgVwFb2hIQtMm87NdgAVfXjnt4OePseqT1tKx+opk=",
        version = "v1.4.0",
    )
    go_repository(
        name = "com_github_go_stack_stack",
        importpath = "github.com/go-stack/stack",
        sum = "h1:ntEHSVwIt7PNXNpgPmVfMrNhLtgjlmnZha2kOpuRiDw=",
        version = "v1.8.1",
    )
    go_repository(
        name = "com_github_go_task_slim_sprig",
        importpath = "github.com/go-task/slim-sprig",
        sum = "h1:tfuBGBXKqDEevZMzYi5KSi8KkcZtzBcTgAUUtapy0OI=",
        version = "v0.0.0-20230315185526-52ccab3ef572",
    )
    go_repository(
        name = "com_github_go_task_slim_sprig_v3",
        importpath = "github.com/go-task/slim-sprig/v3",
        sum = "h1:sUs3vkvUymDpBKi3qH1YSqBQk9+9D/8M2mN1vB6EwHI=",
        version = "v3.0.0",
    )
    go_repository(
        name = "com_github_go_yaml_yaml",
        importpath = "github.com/go-yaml/yaml",
        sum = "h1:RYi2hDdss1u4YE7GwixGzWwVo47T8UQwnTLB6vQiq+o=",
        version = "v2.1.0+incompatible",
    )
    go_repository(
        name = "com_github_goccy_go_json",
        importpath = "github.com/goccy/go-json",
        sum = "h1:CrxCmQqYDkv1z7lO7Wbh2HN93uovUHgrECaO5ZrCXAU=",
        version = "v0.10.2",
    )
    go_repository(
        name = "com_github_godbus_dbus_v5",
        importpath = "github.com/godbus/dbus/v5",
        sum = "h1:4KLkAxT3aOY8Li4FRJe/KvhoNFFxo0m6fNuFUO8QJUk=",
        version = "v5.1.0",
    )
    go_repository(
        name = "com_github_gofrs_flock",
        importpath = "github.com/gofrs/flock",
        sum = "h1:+gYjHKf32LDeiEEFhQaotPbLuUXjY5ZqxKgXy7n59aw=",
        version = "v0.8.1",
    )
    go_repository(
        name = "com_github_gogo_googleapis",
        importpath = "github.com/gogo/googleapis",
        sum = "h1:1Yx4Myt7BxzvUr5ldGSbwYiZG6t9wGBZ+8/fX3Wvtq0=",
        version = "v1.4.1",
    )
    go_repository(
        name = "com_github_gogo_protobuf",
        importpath = "github.com/gogo/protobuf",
        sum = "h1:Ov1cvc58UF3b5XjBnZv7+opcTcQFZebYjWzi34vdm4Q=",
        version = "v1.3.2",
    )
    go_repository(
        name = "com_github_gogo_status",
        importpath = "github.com/gogo/status",
        sum = "h1:+eIkrewn5q6b30y+g/BJINVVdi2xH7je5MPJ3ZPK3JA=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_golang_gddo",
        importpath = "github.com/golang/gddo",
        sum = "h1:HoqgYR60VYu5+0BuG6pjeGp7LKEPZnHt+dUClx9PeIs=",
        version = "v0.0.0-20200528160355-8d077c1d8f4c",
    )
    go_repository(
        name = "com_github_golang_glog",
        importpath = "github.com/golang/glog",
        sum = "h1:OptwRhECazUx5ix5TTWC3EZhsZEHWcYWY4FQHTIubm4=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_golang_groupcache",
        importpath = "github.com/golang/groupcache",
        sum = "h1:oI5xCqsCo564l8iNU+DwB5epxmsaqB+rhGL0m5jtYqE=",
        version = "v0.0.0-20210331224755-41bb18bfe9da",
    )
    go_repository(
        name = "com_github_golang_jwt_jwt_v4",
        importpath = "github.com/golang-jwt/jwt/v4",
        sum = "h1:7cYmW1XlMY7h7ii7UhUyChSgS5wUJEnm9uZVTGqOWzg=",
        version = "v4.5.0",
    )
    go_repository(
        name = "com_github_golang_lint",
        importpath = "github.com/golang/lint",
        sum = "h1:2hRPrmiwPrp3fQX967rNJIhQPtiGXdlQWAxKbKw3VHA=",
        version = "v0.0.0-20180702182130-06c8688daad7",
    )
    go_repository(
        name = "com_github_golang_mock",
        importpath = "github.com/golang/mock",
        sum = "h1:l75CXGRSwbaYNpl/Z2X1XIIAMSCquvXgpVZDhwEIJsc=",
        version = "v1.4.4",
    )
    go_repository(
        name = "com_github_golang_protobuf",
        importpath = "github.com/golang/protobuf",
        patch_args = ["-p1"],
        patches = ["@io_bazel_rules_go//third_party:com_github_golang_protobuf-extras.patch"],
        sum = "h1:i7eJL8qZTpSEXOPTxNKhASYpMn+8e5Q6AdndVa1dWek=",
        version = "v1.5.4",
    )
    go_repository(
        name = "com_github_golang_snappy",
        importpath = "github.com/golang/snappy",
        sum = "h1:PBC98N2aIaM3XXiurYmW7fx4GZkL8feAMVq7nEjURHk=",
        version = "v0.0.5-0.20220116011046-fa5810519dcb",
    )
    go_repository(
        name = "com_github_golangci_lint_1",
        importpath = "github.com/golangci/lint-1",
        sum = "h1:utua3L2IbQJmauC5IXdEA547bcoU5dozgQAfc8Onsg4=",
        version = "v0.0.0-20181222135242-d2cdd8c08219",
    )
    go_repository(
        name = "com_github_google_btree",
        importpath = "github.com/google/btree",
        sum = "h1:gK4Kx5IaGY9CD5sPJ36FHiBJ6ZXl0kilRiiCj+jdYp4=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_google_gnostic_models",
        importpath = "github.com/google/gnostic-models",
        sum = "h1:yo/ABAfM5IMRsS1VnXjTBvUb61tFIHozhlYvRgGre9I=",
        version = "v0.6.8",
    )
    go_repository(
        name = "com_github_google_go_cmp",
        importpath = "github.com/google/go-cmp",
        sum = "h1:ofyhxvXcZhMsU5ulbFiLKl/XBFqE1GSq7atu8tAmTRI=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_github_google_go_github",
        importpath = "github.com/google/go-github",
        sum = "h1:N0LgJ1j65A7kfXrZnUDaYCs/Sf4rEjNlfyDHW9dolSY=",
        version = "v17.0.0+incompatible",
    )
    go_repository(
        name = "com_github_google_go_querystring",
        importpath = "github.com/google/go-querystring",
        sum = "h1:AnCroh3fv4ZBgVIf1Iwtovgjaw/GiKJo8M8yD/fhyJ8=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_google_gofuzz",
        importpath = "github.com/google/gofuzz",
        sum = "h1:xRy4A+RhZaiKjJ1bPfwQ8sedCA+YS2YcCHW6ec7JMi0=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_google_gopacket",
        importpath = "github.com/google/gopacket",
        sum = "h1:ves8RnFZPGiFnTS0uPQStjwru6uO6h+nlr9j6fL7kF8=",
        version = "v1.1.19",
    )
    go_repository(
        name = "com_github_google_martian",
        importpath = "github.com/google/martian",
        sum = "h1:/CP5g8u/VJHijgedC/Legn3BAbAaWPgecwXBIDzw5no=",
        version = "v2.1.0+incompatible",
    )
    go_repository(
        name = "com_github_google_martian_v3",
        importpath = "github.com/google/martian/v3",
        sum = "h1:wCKgOCHuUEVfsaQLpPSJb7VdYCdTVZQAuOdYm1yc/60=",
        version = "v3.1.0",
    )
    go_repository(
        name = "com_github_google_pprof",
        importpath = "github.com/google/pprof",
        sum = "h1:FKHo8hFI3A+7w0aUQuYXQ+6EN5stWmeY/AZqtM8xk9k=",
        version = "v0.0.0-20240727154555-813a5fbdbec8",
    )
    go_repository(
        name = "com_github_google_renameio",
        importpath = "github.com/google/renameio",
        sum = "h1:GOZbcHa3HfsPKPlmyPyN2KEohoMXOhdMbHrvbpl2QaA=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_google_subcommands",
        importpath = "github.com/google/subcommands",
        sum = "h1:vWQspBTo2nEqTUFita5/KeEWlUL8kQObDFbub/EN9oE=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_google_uuid",
        importpath = "github.com/google/uuid",
        sum = "h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_github_googleapis_gax_go",
        importpath = "github.com/googleapis/gax-go",
        sum = "h1:j0GKcs05QVmm7yesiZq2+9cxHkNK9YM6zKx4D2qucQU=",
        version = "v2.0.0+incompatible",
    )
    go_repository(
        name = "com_github_googleapis_gax_go_v2",
        importpath = "github.com/googleapis/gax-go/v2",
        sum = "h1:sjZBwGj9Jlw33ImPtvFviGYvseOtDM7hkSKB7+Tv3SM=",
        version = "v2.0.5",
    )
    go_repository(
        name = "com_github_googleapis_google_cloud_go_testing",
        importpath = "github.com/googleapis/google-cloud-go-testing",
        sum = "h1:tlyzajkF3030q6M8SvmJSemC9DTHL/xaMa18b65+JM4=",
        version = "v0.0.0-20200911160855-bcd43fbb19e8",
    )
    go_repository(
        name = "com_github_gopherjs_gopherjs",
        importpath = "github.com/gopherjs/gopherjs",
        sum = "h1:EGx4pi6eqNxGaHF6qqu48+N2wcFQ5qg5FXgOdqsJ5d8=",
        version = "v0.0.0-20181017120253-0766667cb4d1",
    )
    go_repository(
        name = "com_github_gorilla_context",
        importpath = "github.com/gorilla/context",
        sum = "h1:AWwleXJkX/nhcU9bZSnZoi3h/qGYqQAGhq6zZe/aQW8=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_gorilla_css",
        importpath = "github.com/gorilla/css",
        sum = "h1:BQqNyPTi50JCFMTw/b67hByjMVXZRwGha6wxVGkeihY=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_gorilla_mux",
        importpath = "github.com/gorilla/mux",
        sum = "h1:i40aqfkR1h2SlN9hojwV5ZA91wcXFOvkdNIeFDP5koI=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_github_gorilla_websocket",
        importpath = "github.com/gorilla/websocket",
        sum = "h1:saDtZ6Pbx/0u+bgYQ3q96pZgCzfhKXGPqt7kZ72aNNg=",
        version = "v1.5.3",
    )
    go_repository(
        name = "com_github_gostaticanalysis_comment",
        importpath = "github.com/gostaticanalysis/comment",
        sum = "h1:hlnx5+S2fY9Zo9ePo4AhgYsYHbM2+eAv8m/s1JiCd6Q=",
        version = "v1.4.2",
    )
    go_repository(
        name = "com_github_gostaticanalysis_testutil",
        importpath = "github.com/gostaticanalysis/testutil",
        sum = "h1:d2/eIbH9XjD1fFwD5SHv8x168fjbQ9PB8hvs8DSEC08=",
        version = "v0.3.1-0.20210208050101-bfb5c8eec0e4",
    )
    go_repository(
        name = "com_github_graph_gophers_graphql_go",
        importpath = "github.com/graph-gophers/graphql-go",
        sum = "h1:Eb9x/q6MFpCLz7jBCiP/WTxjSDrYLR1QY41SORZyNJ0=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_gregjones_httpcache",
        importpath = "github.com/gregjones/httpcache",
        sum = "h1:pdN6V1QBWetyv/0+wjACpqVH+eVULgEjkurDLq3goeM=",
        version = "v0.0.0-20180305231024-9cad4c3443a7",
    )
    go_repository(
        name = "com_github_grpc_ecosystem_go_grpc_middleware",
        importpath = "github.com/grpc-ecosystem/go-grpc-middleware",
        sum = "h1:FlFbCRLd5Jr4iYXZufAvgWN6Ao0JrI5chLINnUXDDr0=",
        version = "v1.2.2",
    )
    go_repository(
        name = "com_github_grpc_ecosystem_go_grpc_prometheus",
        importpath = "github.com/grpc-ecosystem/go-grpc-prometheus",
        sum = "h1:Ovs26xHkKqVztRpIrF/92BcuyuQ/YW4NSIpoGtfXNho=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_grpc_ecosystem_grpc_gateway",
        importpath = "github.com/grpc-ecosystem/grpc-gateway",
        sum = "h1:UImYN5qQ8tuGpGE16ZmjvcTtTw24zw1QAp/SlnNrZhI=",
        version = "v1.9.5",
    )
    go_repository(
        name = "com_github_guptarohit_asciigraph",
        importpath = "github.com/guptarohit/asciigraph",
        sum = "h1:ccFnUF8xYIOUPPY3tmdvRyHqmn1MYI9iv1pLKX+/ZkQ=",
        version = "v0.5.5",
    )
    go_repository(
        name = "com_github_hashicorp_consul_api",
        importpath = "github.com/hashicorp/consul/api",
        sum = "h1:HXNYlRkkM/t+Y/Yhxtwcy02dlYwIaoxzvxPnS+cqy78=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_hashicorp_consul_sdk",
        importpath = "github.com/hashicorp/consul/sdk",
        sum = "h1:UOxjlb4xVNF93jak1mzzoBatyFju9nrkxpVwIp/QqxQ=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_github_hashicorp_errwrap",
        importpath = "github.com/hashicorp/errwrap",
        sum = "h1:hLrqtEDnRye3+sgx6z4qVLNuviH3MR5aQ0ykNJa/UYA=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_bexpr",
        importpath = "github.com/hashicorp/go-bexpr",
        sum = "h1:9kuI5PFotCboP3dkDYFr/wi0gg0QVbSNz5oFRpxn4uE=",
        version = "v0.1.10",
    )
    go_repository(
        name = "com_github_hashicorp_go_cleanhttp",
        importpath = "github.com/hashicorp/go-cleanhttp",
        sum = "h1:035FKYIWjmULyFRBKPs8TBQoi0x6d9G4xc9neXJWAZQ=",
        version = "v0.5.2",
    )
    go_repository(
        name = "com_github_hashicorp_go_immutable_radix",
        importpath = "github.com/hashicorp/go-immutable-radix",
        sum = "h1:AKDB1HM5PWEA7i4nhcpwOrO2byshxBjXVn/J/3+z5/0=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_msgpack",
        importpath = "github.com/hashicorp/go-msgpack",
        sum = "h1:zKjpN5BK/P5lMYrLmBHdBULWbJ0XpYR+7NGzqkZzoD4=",
        version = "v0.5.3",
    )
    go_repository(
        name = "com_github_hashicorp_go_multierror",
        importpath = "github.com/hashicorp/go-multierror",
        sum = "h1:iVjPR7a6H0tWELX5NxNe7bYopibicUzc7uPribsnS6o=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_net",
        importpath = "github.com/hashicorp/go.net",
        sum = "h1:sNCoNyDEvN1xa+X0baata4RdcpKwcMS6DH+xwfqPgjw=",
        version = "v0.0.1",
    )
    go_repository(
        name = "com_github_hashicorp_go_retryablehttp",
        importpath = "github.com/hashicorp/go-retryablehttp",
        sum = "h1:ZQgVdpTdAL7WpMIwLzCfbalOcSUdkDZnpUv3/+BxzFA=",
        version = "v0.7.4",
    )
    go_repository(
        name = "com_github_hashicorp_go_rootcerts",
        importpath = "github.com/hashicorp/go-rootcerts",
        sum = "h1:Rqb66Oo1X/eSV1x66xbDccZjhJigjg0+e82kpwzSwCI=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_sockaddr",
        importpath = "github.com/hashicorp/go-sockaddr",
        sum = "h1:GeH6tui99pF4NJgfnhp+L6+FfobzVW3Ah46sLo0ICXs=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_syslog",
        importpath = "github.com/hashicorp/go-syslog",
        sum = "h1:KaodqZuhUoZereWVIYmpUgZysurB1kBLX2j0MwMrUAE=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_uuid",
        importpath = "github.com/hashicorp/go-uuid",
        sum = "h1:cfejS+Tpcp13yd5nYHWDI6qVCny6wyX2Mt5SGur2IGE=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_hashicorp_go_version",
        importpath = "github.com/hashicorp/go-version",
        sum = "h1:zEfKbn2+PDgroKdiOzqiE8rsmLqU2uwi5PB5pBJ3TkI=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_hashicorp_golang_lru",
        importpath = "github.com/hashicorp/golang-lru",
        sum = "h1:dg1dEPuWpEqDnvIw251EVy4zlP8gWbsGj4BsUKCRpYs=",
        version = "v0.5.5-0.20210104140557-80c98217689d",
    )
    go_repository(
        name = "com_github_hashicorp_golang_lru_arc_v2",
        importpath = "github.com/hashicorp/golang-lru/arc/v2",
        sum = "h1:QxkVTxwColcduO+LP7eJO56r2hFiG8zEbfAAzRv52KQ=",
        version = "v2.0.7",
    )
    go_repository(
        name = "com_github_hashicorp_golang_lru_v2",
        importpath = "github.com/hashicorp/golang-lru/v2",
        sum = "h1:a+bsQ5rvGLjzHuww6tVxozPZFVghXaHOwFs4luLUK2k=",
        version = "v2.0.7",
    )
    go_repository(
        name = "com_github_hashicorp_hcl",
        importpath = "github.com/hashicorp/hcl",
        sum = "h1:LFTfzwAUSKPijQbJrMWZm/CysECsF/U1UUniUeXxzFw=",
        version = "v0.0.0-20170914154624-68e816d1c783",
    )
    go_repository(
        name = "com_github_hashicorp_logutils",
        importpath = "github.com/hashicorp/logutils",
        sum = "h1:dLEQVugN8vlakKOUE3ihGLTZJRB4j+M2cdTm/ORI65Y=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_mdns",
        importpath = "github.com/hashicorp/mdns",
        sum = "h1:WhIgCr5a7AaVH6jPUwjtRuuE7/RDufnUvzIr48smyxs=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hashicorp_memberlist",
        importpath = "github.com/hashicorp/memberlist",
        sum = "h1:EmmoJme1matNzb+hMpDuR/0sbJSUisxyqBGG676r31M=",
        version = "v0.1.3",
    )
    go_repository(
        name = "com_github_hashicorp_serf",
        importpath = "github.com/hashicorp/serf",
        sum = "h1:YZ7UKsJv+hKjqGVUUbtE3HNj79Eln2oQ75tniF6iPt0=",
        version = "v0.8.2",
    )
    go_repository(
        name = "com_github_hdrhistogram_hdrhistogram_go",
        importpath = "github.com/HdrHistogram/hdrhistogram-go",
        sum = "h1:5IcZpTvzydCQeHzK4Ef/D5rrSqwxob0t8PQPMybUNFM=",
        version = "v1.1.2",
    )
    go_repository(
        name = "com_github_herumi_bls_eth_go_binary",
        importpath = "github.com/herumi/bls-eth-go-binary",
        sum = "h1:wCMygKUQhmcQAjlk2Gquzq6dLmyMv2kF+llRspoRgrk=",
        version = "v0.0.0-20210917013441-d37c07cfda4e",
    )
    go_repository(
        name = "com_github_holiman_billy",
        importpath = "github.com/holiman/billy",
        sum = "h1:3JQNjnMRil1yD0IfZKHF9GxxWKDJGj8I0IqOUol//sw=",
        version = "v0.0.0-20230718173358-1c7e68d277a7",
    )
    go_repository(
        name = "com_github_holiman_bloomfilter_v2",
        importpath = "github.com/holiman/bloomfilter/v2",
        sum = "h1:73e0e/V0tCydx14a0SCYS/EWCxgwLZ18CZcZKVu0fao=",
        version = "v2.0.3",
    )
    go_repository(
        name = "com_github_holiman_goevmlab",
        importpath = "github.com/holiman/goevmlab",
        sum = "h1:J973NLskKmFIj3EGfpQ1ztUQKdwyJ+fG34638ief0eA=",
        version = "v0.0.0-20231201084119-c73b3c97929c",
    )
    go_repository(
        name = "com_github_holiman_uint256",
        importpath = "github.com/holiman/uint256",
        sum = "h1:jUc4Nk8fm9jZabQuqr2JzednajVmBpC+oiTiXZJEApU=",
        version = "v1.2.4",
    )
    go_repository(
        name = "com_github_hpcloud_tail",
        importpath = "github.com/hpcloud/tail",
        sum = "h1:nfCOvKYfkgYP8hkirhJocXT2+zOD8yUNjXaWfTlyFKI=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_hudl_fargo",
        importpath = "github.com/hudl/fargo",
        sum = "h1:0U6+BtN6LhaYuTnIJq4Wyq5cpn6O2kWrxAtcqBmYY6w=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_huin_goupnp",
        importpath = "github.com/huin/goupnp",
        sum = "h1:UvLUlWDNpoUdYzb2TCn+MuTWtcjXKSza2n6CBdQ0xXc=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_hydrogen18_memlistener",
        importpath = "github.com/hydrogen18/memlistener",
        sum = "h1:JR7eDj8HD6eXrc5fWLbSUnfcQFL06PYvCc0DKQnWfaU=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_ianlancetaylor_cgosymbolizer",
        importpath = "github.com/ianlancetaylor/cgosymbolizer",
        sum = "h1:IpTHAzWv1pKDDWeJDY5VOHvqc2T9d3C8cPKEf2VPqHE=",
        version = "v0.0.0-20200424224625-be1b05b0b279",
    )
    go_repository(
        name = "com_github_ianlancetaylor_demangle",
        importpath = "github.com/ianlancetaylor/demangle",
        sum = "h1:KwWnWVWCNtNq/ewIX7HIKnELmEx2nDP42yskD/pi7QE=",
        version = "v0.0.0-20240312041847-bd984b5ce465",
    )
    go_repository(
        name = "com_github_imdario_mergo",
        importpath = "github.com/imdario/mergo",
        sum = "h1:xTNEAn+kxVO7dTZGu0CegyqKZmoWFI0rF8UxjlB2d28=",
        version = "v0.3.6",
    )
    go_repository(
        name = "com_github_inconshreveable_log15",
        importpath = "github.com/inconshreveable/log15",
        sum = "h1:g/SJtZVYc1cxSB8lgrgqeOlIdi4MhqNNHYRAC8y+g4c=",
        version = "v0.0.0-20170622235902-74a0988b5f80",
    )
    go_repository(
        name = "com_github_inconshreveable_mousetrap",
        importpath = "github.com/inconshreveable/mousetrap",
        sum = "h1:Z8tu5sraLXCXIcARxBp/8cbvlwVa7Z1NHg9XEKhtSvM=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_influxdata_influxdb1_client",
        importpath = "github.com/influxdata/influxdb1-client",
        sum = "h1:qSHzRbhzK8RdXOsAdfDgO49TtqC1oZ+acxPrkfTxcCs=",
        version = "v0.0.0-20220302092344-a9ab5670611c",
    )
    go_repository(
        name = "com_github_influxdata_influxdb_client_go_v2",
        importpath = "github.com/influxdata/influxdb-client-go/v2",
        sum = "h1:HGBfZYStlx3Kqvsv1h2pJixbCl/jhnFtxpKFAv9Tu5k=",
        version = "v2.4.0",
    )
    go_repository(
        name = "com_github_influxdata_line_protocol",
        importpath = "github.com/influxdata/line-protocol",
        sum = "h1:vilfsDSy7TDxedi9gyBkMvAirat/oRcL0lFdJBf6tdM=",
        version = "v0.0.0-20210311194329-9aa0e372d097",
    )
    go_repository(
        name = "com_github_ipfs_go_cid",
        importpath = "github.com/ipfs/go-cid",
        sum = "h1:A/T3qGvxi4kpKWWcPC/PgbvDA2bjVLO7n4UeVwnbs/s=",
        version = "v0.4.1",
    )
    go_repository(
        name = "com_github_ipfs_go_datastore",
        importpath = "github.com/ipfs/go-datastore",
        sum = "h1:JKyz+Gvz1QEZw0LsX1IBn+JFCJQH4SJVFtM4uWU0Myk=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_github_ipfs_go_ds_badger",
        importpath = "github.com/ipfs/go-ds-badger",
        sum = "h1:xREL3V0EH9S219kFFueOYJJTcjgNSZ2HY1iSvN7U1Ro=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_github_ipfs_go_ds_leveldb",
        importpath = "github.com/ipfs/go-ds-leveldb",
        sum = "h1:s++MEBbD3ZKc9/8/njrn4flZLnCuY9I79v94gBUNumo=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_github_ipfs_go_log_v2",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/ipfs/go-log/v2",
        sum = "h1:1XdUzF7048prq4aBjDQQ4SL5RxftpRGdXhNRwKSAlcY=",
        version = "v2.5.1",
    )
    go_repository(
        name = "com_github_iris_contrib_schema",
        importpath = "github.com/iris-contrib/schema",
        sum = "h1:CPSBLyx2e91H2yJzPuhGuifVRnZBBJ3pCOMbOvPZaTw=",
        version = "v0.0.6",
    )
    go_repository(
        name = "com_github_jackpal_go_nat_pmp",
        importpath = "github.com/jackpal/go-nat-pmp",
        sum = "h1:KzKSgb7qkJvOUTqYl9/Hg/me3pWgBmERKrTGD7BdWus=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_jbenet_go_temp_err_catcher",
        importpath = "github.com/jbenet/go-temp-err-catcher",
        sum = "h1:zpb3ZH6wIE8Shj2sKS+khgRvf7T7RABoLk/+KKHggpk=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_jbenet_goprocess",
        importpath = "github.com/jbenet/goprocess",
        sum = "h1:DRGOFReOMqqDNXwW70QkacFW0YN9QnwLV0Vqk+3oU0o=",
        version = "v0.1.4",
    )
    go_repository(
        name = "com_github_jcmturner_gofork",
        importpath = "github.com/jcmturner/gofork",
        sum = "h1:J7uCkflzTEhUZ64xqKnkDxq3kzc96ajM1Gli5ktUem8=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_jedib0t_go_pretty_v6",
        importpath = "github.com/jedib0t/go-pretty/v6",
        sum = "h1:gOGo0613MoqUcf0xCj+h/V3sHDaZasfv152G6/5l91s=",
        version = "v6.5.4",
    )
    go_repository(
        name = "com_github_jedisct1_go_minisign",
        importpath = "github.com/jedisct1/go-minisign",
        sum = "h1:TMtDYDHKYY15rFihtRfck/bfFqNfvcabqvXAFQfAUpY=",
        version = "v0.0.0-20230811132847-661be99b8267",
    )
    go_repository(
        name = "com_github_jellevandenhooff_dkim",
        importpath = "github.com/jellevandenhooff/dkim",
        sum = "h1:ujPKutqRlJtcfWk6toYVYagwra7HQHbXOaS171b4Tg8=",
        version = "v0.0.0-20150330215556-f50fe3d243e1",
    )
    go_repository(
        name = "com_github_jmespath_go_jmespath",
        importpath = "github.com/jmespath/go-jmespath",
        sum = "h1:BEgLn5cpjn8UN1mAw4NjwDrS35OdebyEtFe+9YPoQUg=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_joker_jade",
        importpath = "github.com/Joker/jade",
        sum = "h1:Qbeh12Vq6BxURXT1qZBRHsDxeURB8ztcL6f3EXSGeHk=",
        version = "v1.1.3",
    )
    go_repository(
        name = "com_github_jonboulle_clockwork",
        importpath = "github.com/jonboulle/clockwork",
        sum = "h1:VKV+ZcuP6l3yW9doeqz6ziZGgcynBVQO+obU0+0hcPo=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_joonix_log",
        importpath = "github.com/joonix/log",
        sum = "h1:k+SfYbN66Ev/GDVq39wYOXVW5RNd5kzzairbCe9dK5Q=",
        version = "v0.0.0-20200409080653-9c1d2ceb5f1d",
    )
    go_repository(
        name = "com_github_josharian_intern",
        importpath = "github.com/josharian/intern",
        sum = "h1:vlS4z54oSdjm0bgjRigI+G1HpF+tI+9rE5LLzOg8HmY=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_jpillora_backoff",
        importpath = "github.com/jpillora/backoff",
        sum = "h1:uvFg412JmmHBHw7iwprIxkPMI+sGQ4kzOWsMeHnm2EA=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_json_iterator_go",
        importpath = "github.com/json-iterator/go",
        replace = "github.com/prestonvanloon/go",
        sum = "h1:Bt5PzQCqfP4xiLXDSrMoqAfj6CBr3N9DAyyq8OiIWsc=",
        version = "v1.1.7-0.20190722034630-4f2e55fcf87b",
    )
    go_repository(
        name = "com_github_jstemmer_go_junit_report",
        importpath = "github.com/jstemmer/go-junit-report",
        sum = "h1:6QPYqodiu3GuPL+7mfx+NwDdp2eTkp9IfEUpgAwUN0o=",
        version = "v0.9.1",
    )
    go_repository(
        name = "com_github_jtolds_gls",
        importpath = "github.com/jtolds/gls",
        sum = "h1:xdiiI2gbIgH/gLH7ADydsJ1uDOEzR8yvV7C0MuV77Wo=",
        version = "v4.20.0+incompatible",
    )
    go_repository(
        name = "com_github_juju_ansiterm",
        importpath = "github.com/juju/ansiterm",
        sum = "h1:FaWFmfWdAUKbSCtOU2QjDaorUexogfaMgbipgYATUMU=",
        version = "v0.0.0-20180109212912-720a0952cc2a",
    )
    go_repository(
        name = "com_github_julienschmidt_httprouter",
        importpath = "github.com/julienschmidt/httprouter",
        sum = "h1:U0609e9tgbseu3rBINet9P48AI/D3oJs4dN7jwJOQ1U=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_k0kubun_go_ansi",
        importpath = "github.com/k0kubun/go-ansi",
        sum = "h1:qGQQKEcAR99REcMpsXCp3lJ03zYT1PkRd3kQGPn9GVg=",
        version = "v0.0.0-20180517002512-3bf9e2903213",
    )
    go_repository(
        name = "com_github_karalabe_usb",
        importpath = "github.com/karalabe/usb",
        sum = "h1:AqsttAyEyIEsNz5WLRwuRwjiT5CMDUfLk6cFJDVPebs=",
        version = "v0.0.3-0.20230711191512-61db3e06439c",
    )
    go_repository(
        name = "com_github_kataras_blocks",
        importpath = "github.com/kataras/blocks",
        sum = "h1:cF3RDY/vxnSRezc7vLFlQFTYXG/yAr1o7WImJuZbzC4=",
        version = "v0.0.7",
    )
    go_repository(
        name = "com_github_kataras_golog",
        importpath = "github.com/kataras/golog",
        sum = "h1:isP8th4PJH2SrbkciKnylaND9xoTtfxv++NB+DF0l9g=",
        version = "v0.1.8",
    )
    go_repository(
        name = "com_github_kataras_iris_v12",
        importpath = "github.com/kataras/iris/v12",
        sum = "h1:WzDY5nGuW/LgVaFS5BtTkW3crdSKJ/FEgWnxPnIVVLI=",
        version = "v12.2.0",
    )
    go_repository(
        name = "com_github_kataras_pio",
        importpath = "github.com/kataras/pio",
        sum = "h1:kqreJ5KOEXGMwHAWHDwIl+mjfNCPhAwZPa8gK7MKlyw=",
        version = "v0.0.11",
    )
    go_repository(
        name = "com_github_kataras_sitemap",
        importpath = "github.com/kataras/sitemap",
        sum = "h1:w71CRMMKYMJh6LR2wTgnk5hSgjVNB9KL60n5e2KHvLY=",
        version = "v0.0.6",
    )
    go_repository(
        name = "com_github_kataras_tunnel",
        importpath = "github.com/kataras/tunnel",
        sum = "h1:sCAqWuJV7nPzGrlb0os3j49lk2JhILT0rID38NHNLpA=",
        version = "v0.0.4",
    )
    go_repository(
        name = "com_github_kilic_bls12_381",
        importpath = "github.com/kilic/bls12-381",
        sum = "h1:encrdjqKMEvabVQ7qYOKu1OvhqpK4s47wDYtNiPtlp4=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_kisielk_errcheck",
        importpath = "github.com/kisielk/errcheck",
        sum = "h1:e8esj/e4R+SAOwFwN+n3zr0nYeCyeweozKfO23MvHzY=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_github_kisielk_gotool",
        importpath = "github.com/kisielk/gotool",
        sum = "h1:AV2c/EiW3KqPNT9ZKl07ehoAGi4C5/01Cfbblndcapg=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_klauspost_compress",
        importpath = "github.com/klauspost/compress",
        sum = "h1:6KIumPrER1LHsvBVuDa0r5xaG0Es51mhhB9BQB2qeMA=",
        version = "v1.17.9",
    )
    go_repository(
        name = "com_github_klauspost_cpuid",
        importpath = "github.com/klauspost/cpuid",
        sum = "h1:CCtW0xUnWGVINKvE/WWOYKdsPV6mawAtvQuSl8guwQs=",
        version = "v1.2.3",
    )
    go_repository(
        name = "com_github_klauspost_cpuid_v2",
        importpath = "github.com/klauspost/cpuid/v2",
        sum = "h1:+StwCXwm9PdpiEkPyzBXIy+M9KUb4ODm0Zarf1kS5BM=",
        version = "v2.2.8",
    )
    go_repository(
        name = "com_github_klauspost_reedsolomon",
        importpath = "github.com/klauspost/reedsolomon",
        sum = "h1:N/VzgeMfHmLc+KHMD1UL/tNkfXAt8FnUqlgXGIduwAY=",
        version = "v1.9.3",
    )
    go_repository(
        name = "com_github_knetic_govaluate",
        importpath = "github.com/Knetic/govaluate",
        sum = "h1:1G1pk05UrOh0NlF1oeaaix1x8XzrfjIDK47TY0Zehcw=",
        version = "v3.0.1-0.20171022003610-9aa49832a739+incompatible",
    )
    go_repository(
        name = "com_github_konsorten_go_windows_terminal_sequences",
        importpath = "github.com/konsorten/go-windows-terminal-sequences",
        sum = "h1:CE8S1cTafDpPvMhIxNJKvHsGVBgn1xWYf1NbHQhywc8=",
        version = "v1.0.3",
    )
    go_repository(
        name = "com_github_koron_go_ssdp",
        importpath = "github.com/koron/go-ssdp",
        sum = "h1:1IDwrghSKYM7yLf7XCzbByg2sJ/JcNOZRXS2jczTwz0=",
        version = "v0.0.4",
    )
    go_repository(
        name = "com_github_korovkin_limiter",
        importpath = "github.com/korovkin/limiter",
        sum = "h1:7CfsXfFpCG1wrUpuyOzG8+vpL1ZqH2goz23wZ9pboGE=",
        version = "v0.0.0-20230307205149-3d4b2b34c99d",
    )
    go_repository(
        name = "com_github_kr_fs",
        importpath = "github.com/kr/fs",
        sum = "h1:Jskdu9ieNAYnjxsi0LbQp1ulIKZV1LAFgK1tWhpZgl8=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_kr_logfmt",
        importpath = "github.com/kr/logfmt",
        sum = "h1:T+h1c/A9Gawja4Y9mFVWj2vyii2bbUNDw3kt9VxK2EY=",
        version = "v0.0.0-20140226030751-b84e30acd515",
    )
    go_repository(
        name = "com_github_kr_pretty",
        importpath = "github.com/kr/pretty",
        sum = "h1:flRD4NNwYAUpkphVc1HcthR4KEIFJ65n8Mw5qdRn3LE=",
        version = "v0.3.1",
    )
    go_repository(
        name = "com_github_kr_pty",
        importpath = "github.com/kr/pty",
        sum = "h1:/Um6a/ZmD5tF7peoOJ5oN5KMQ0DrGVQSXLNwyckutPk=",
        version = "v1.1.3",
    )
    go_repository(
        name = "com_github_kr_text",
        importpath = "github.com/kr/text",
        sum = "h1:5Nx0Ya0ZqY2ygV366QzturHI13Jq95ApcVaJBhpS+AY=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_kylelemons_godebug",
        importpath = "github.com/kylelemons/godebug",
        sum = "h1:RPNrshWIDI6G2gRW9EHilWtl7Z6Sb1BR0xunSBf0SNc=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_labstack_echo_v4",
        importpath = "github.com/labstack/echo/v4",
        sum = "h1:5CiyngihEO4HXsz3vVsJn7f8xAlWwRr3aY6Ih280ZKA=",
        version = "v4.10.0",
    )
    go_repository(
        name = "com_github_labstack_gommon",
        importpath = "github.com/labstack/gommon",
        sum = "h1:y7cvthEAEbU0yHOf4axH8ZG2NH8knB9iNSoTO8dyIk8=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_leanovate_gopter",
        importpath = "github.com/leanovate/gopter",
        sum = "h1:fQjYxZaynp97ozCzfOyOuAGOU4aU/z37zf/tOujFk7c=",
        version = "v0.2.9",
    )
    go_repository(
        name = "com_github_leodido_go_urn",
        importpath = "github.com/leodido/go-urn",
        sum = "h1:6BE2vPT0lqoz3fmOesHZiaiFh7889ssCo2GMvLCfiuA=",
        version = "v1.2.3",
    )
    go_repository(
        name = "com_github_libp2p_go_buffer_pool",
        importpath = "github.com/libp2p/go-buffer-pool",
        sum = "h1:oK4mSFcQz7cTQIfqbe4MIj9gLW+mnanjyFtc6cdF0Y8=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_libp2p_go_flow_metrics",
        importpath = "github.com/libp2p/go-flow-metrics",
        sum = "h1:0iPhMI8PskQwzh57jB9WxIuIOQ0r+15PChFGkx3Q3WM=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_libp2p_go_libp2p",
        build_directives = [
            "gazelle:exclude tools.go",
        ],
        build_file_proto_mode = "disable_global",
        importpath = "github.com/libp2p/go-libp2p",
        sum = "h1:DoABsaHO0VXwH6pwCs2F6XKAXWYjFMO4HFBoVxTnF9g=",
        version = "v0.36.5",
    )
    go_repository(
        name = "com_github_libp2p_go_libp2p_asn_util",
        importpath = "github.com/libp2p/go-libp2p-asn-util",
        sum = "h1:xqL7++IKD9TBFMgnLPZR6/6iYhawHKHl950SO9L6n94=",
        version = "v0.4.1",
    )
    go_repository(
        name = "com_github_libp2p_go_libp2p_mplex",
        importpath = "github.com/libp2p/go-libp2p-mplex",
        sum = "h1:R58pDRAmuBXkYugbSSXR9wrTX3+1pFM1xP2bLuodIq8=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_github_libp2p_go_libp2p_pubsub",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/libp2p/go-libp2p-pubsub",
        sum = "h1:PENNZjSfk8KYxANRlpipdS7+BfLmOl3L2E/6vSNjbdI=",
        version = "v0.12.0",
    )
    go_repository(
        name = "com_github_libp2p_go_libp2p_testing",
        importpath = "github.com/libp2p/go-libp2p-testing",
        sum = "h1:EPvBb4kKMWO29qP4mZGyhVzUyR25dvfUIK5WDu6iPUA=",
        version = "v0.12.0",
    )
    go_repository(
        name = "com_github_libp2p_go_mplex",
        importpath = "github.com/libp2p/go-mplex",
        sum = "h1:BDhFZdlk5tbr0oyFq/xv/NPGfjbnrsDam1EvutpBDbY=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_github_libp2p_go_msgio",
        importpath = "github.com/libp2p/go-msgio",
        sum = "h1:mf3Z8B1xcFN314sWX+2vOTShIE0Mmn2TXn3YCUQGNj0=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_github_libp2p_go_nat",
        importpath = "github.com/libp2p/go-nat",
        sum = "h1:Tyz+bUFAYqGyJ/ppPPymMGbIgNRH+WqC5QrT5fKrrGk=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_libp2p_go_netroute",
        importpath = "github.com/libp2p/go-netroute",
        sum = "h1:V8kVrpD8GK0Riv15/7VN6RbUQ3URNZVosw7H2v9tksU=",
        version = "v0.2.1",
    )
    go_repository(
        name = "com_github_libp2p_go_reuseport",
        importpath = "github.com/libp2p/go-reuseport",
        sum = "h1:nR5KU7hD0WxXCJbmw7r2rhRYruNRl2koHw8fQscQm2s=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_libp2p_go_yamux_v4",
        importpath = "github.com/libp2p/go-yamux/v4",
        sum = "h1:FfDR4S1wj6Bw2Pqbc8Uz7pCxeRBPbwsBbEdfwiCypkQ=",
        version = "v4.0.1",
    )
    go_repository(
        name = "com_github_libp2p_zeroconf_v2",
        importpath = "github.com/libp2p/zeroconf/v2",
        sum = "h1:Cup06Jv6u81HLhIj1KasuNM/RHHrJ8T7wOTS4+Tv53Q=",
        version = "v2.2.0",
    )
    go_repository(
        name = "com_github_lightstep_lightstep_tracer_common_golang_gogo",
        importpath = "github.com/lightstep/lightstep-tracer-common/golang/gogo",
        sum = "h1:143Bb8f8DuGWck/xpNUOckBVYfFbBTnLevfRZ1aVVqo=",
        version = "v0.0.0-20190605223551-bc2310a04743",
    )
    go_repository(
        name = "com_github_lightstep_lightstep_tracer_go",
        importpath = "github.com/lightstep/lightstep-tracer-go",
        sum = "h1:vi1F1IQ8N7hNWytK9DpJsUfQhGuNSc19z330K6vl4zk=",
        version = "v0.18.1",
    )
    go_repository(
        name = "com_github_logrusorgru_aurora",
        importpath = "github.com/logrusorgru/aurora",
        sum = "h1:tOpm7WcpBTn4fjmVfgpQq0EfczGlG91VSDkswnjF5A8=",
        version = "v2.0.3+incompatible",
    )
    go_repository(
        name = "com_github_lucasb_eyer_go_colorful",
        importpath = "github.com/lucasb-eyer/go-colorful",
        sum = "h1:1nnpGOrhyZZuNyfu1QjKiUICQ74+3FNCN69Aj6K7nkY=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_lunixbochs_vtclean",
        importpath = "github.com/lunixbochs/vtclean",
        sum = "h1:xu2sLAri4lGiovBDQKxl5mrXyESr3gUr5m5SM5+LVb8=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_lyft_protoc_gen_validate",
        importpath = "github.com/lyft/protoc-gen-validate",
        sum = "h1:KNt/RhmQTOLr7Aj8PsJ7mTronaFyx80mRTT9qF261dA=",
        version = "v0.0.13",
    )
    go_repository(
        name = "com_github_magiconair_properties",
        importpath = "github.com/magiconair/properties",
        sum = "h1:SesWF0c8l/IKQX0NlsED38qoBhUpneg5HIHNdy5LyEE=",
        version = "v1.7.4-0.20170902060319-8d7837e64d3c",
    )
    go_repository(
        name = "com_github_mailgun_raymond_v2",
        importpath = "github.com/mailgun/raymond/v2",
        sum = "h1:5dmlB680ZkFG2RN/0lvTAghrSxIESeu9/2aeDqACtjw=",
        version = "v2.0.48",
    )
    go_repository(
        name = "com_github_mailru_easyjson",
        importpath = "github.com/mailru/easyjson",
        sum = "h1:UGYAvKxe3sBsEDzO8ZeWOSlIQfWFlxbzLZe7hwFURr0=",
        version = "v0.7.7",
    )
    go_repository(
        name = "com_github_manifoldco_promptui",
        importpath = "github.com/manifoldco/promptui",
        sum = "h1:3l11YT8tm9MnwGFQ4kETwkzpAwY2Jt9lCrumCUW4+z4=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_github_mariusvanderwijden_fuzzyvm",
        importpath = "github.com/MariusVanDerWijden/FuzzyVM",
        sum = "h1:BwEuC3xavrv4HTUDH2fUrKgKooiH3Q/nSVnFGtnzpN0=",
        version = "v0.0.0-20240209103030-ec53fa766bf8",
    )
    go_repository(
        name = "com_github_mariusvanderwijden_tx_fuzz",
        importpath = "github.com/MariusVanDerWijden/tx-fuzz",
        sum = "h1:QDTh0xHorSykJ4+2VccBADMeRAVUbnHaWrCPIMtN+Vc=",
        version = "v1.3.3-0.20240227085032-f70dd7c85c97",
    )
    go_repository(
        name = "com_github_marten_seemann_tcp",
        importpath = "github.com/marten-seemann/tcp",
        sum = "h1:br0buuQ854V8u83wA0rVZ8ttrq5CpaPZdvrK0LP2lOk=",
        version = "v0.0.0-20210406111302-dfbc87cc63fd",
    )
    go_repository(
        name = "com_github_matryer_moq",
        importpath = "github.com/matryer/moq",
        sum = "h1:HvFwW+cm9bCbZ/+vuGNq7CRWXql8c0y8nGeYpqmpvmk=",
        version = "v0.0.0-20190312154309-6cfb0558e1bd",
    )
    go_repository(
        name = "com_github_mattn_go_colorable",
        importpath = "github.com/mattn/go-colorable",
        sum = "h1:fFA4WZxdEF4tXPZVKMLwD8oUnCTTo08duU7wxecdEvA=",
        version = "v0.1.13",
    )
    go_repository(
        name = "com_github_mattn_go_isatty",
        importpath = "github.com/mattn/go-isatty",
        sum = "h1:xfD0iDuEKnDkl03q4limB+vH+GxLEtL/jb4xVJSWWEY=",
        version = "v0.0.20",
    )
    go_repository(
        name = "com_github_mattn_go_runewidth",
        importpath = "github.com/mattn/go-runewidth",
        sum = "h1:UNAjwbU9l54TA3KzvqLGxwWjHmMgBUVhBiTjelZgg3U=",
        version = "v0.0.15",
    )
    go_repository(
        name = "com_github_matttproud_golang_protobuf_extensions",
        importpath = "github.com/matttproud/golang_protobuf_extensions",
        sum = "h1:mmDVorXM7PCGKw94cs5zkfA9PSy5pEvNWRP0ET0TIVo=",
        version = "v1.0.4",
    )
    go_repository(
        name = "com_github_matttproud_golang_protobuf_extensions_v2",
        importpath = "github.com/matttproud/golang_protobuf_extensions/v2",
        sum = "h1:jWpvCLoY8Z/e3VKvlsiIGKtc+UG6U5vzxaoagmhXfyg=",
        version = "v2.0.0",
    )
    go_repository(
        name = "com_github_mgutz_ansi",
        importpath = "github.com/mgutz/ansi",
        sum = "h1:j7+1HpAFS1zy5+Q4qx1fWh90gTKwiN4QCGoY9TWyyO4=",
        version = "v0.0.0-20170206155736-9520e82c474b",
    )
    go_repository(
        name = "com_github_microcosm_cc_bluemonday",
        importpath = "github.com/microcosm-cc/bluemonday",
        sum = "h1:SMZe2IGa0NuHvnVNAZ+6B38gsTbi5e4sViiWJyDDqFY=",
        version = "v1.0.23",
    )
    go_repository(
        name = "com_github_microsoft_go_winio",
        importpath = "github.com/Microsoft/go-winio",
        sum = "h1:9/kr64B9VUZrLm5YYwbGtUJnMgqWVOdUAXu6Migciow=",
        version = "v0.6.1",
    )
    go_repository(
        name = "com_github_miekg_dns",
        importpath = "github.com/miekg/dns",
        sum = "h1:cN8OuEF1/x5Rq6Np+h1epln8OiyPWV+lROx9LxcGgIQ=",
        version = "v1.1.62",
    )
    go_repository(
        name = "com_github_mikioh_tcp",
        importpath = "github.com/mikioh/tcp",
        sum = "h1:bzE/A84HN25pxAuk9Eej1Kz9OUelF97nAc82bDquQI8=",
        version = "v0.0.0-20190314235350-803a9b46060c",
    )
    go_repository(
        name = "com_github_mikioh_tcpinfo",
        importpath = "github.com/mikioh/tcpinfo",
        sum = "h1:z78hV3sbSMAUoyUMM0I83AUIT6Hu17AWfgjzIbtrYFc=",
        version = "v0.0.0-20190314235526-30a79bb1804b",
    )
    go_repository(
        name = "com_github_mikioh_tcpopt",
        importpath = "github.com/mikioh/tcpopt",
        sum = "h1:PTfri+PuQmWDqERdnNMiD9ZejrlswWrCpBEZgWOiTrc=",
        version = "v0.0.0-20190314235656-172688c1accc",
    )
    go_repository(
        name = "com_github_minio_blake2b_simd",
        importpath = "github.com/minio/blake2b-simd",
        sum = "h1:lYpkrQH5ajf0OXOcUbGjvZxxijuBwbbmlSxLiuofa+g=",
        version = "v0.0.0-20160723061019-3f5f724cb5b1",
    )
    go_repository(
        name = "com_github_minio_highwayhash",
        importpath = "github.com/minio/highwayhash",
        sum = "h1:Aak5U0nElisjDCfPSG79Tgzkn2gl66NxOMspRrKnA/g=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_minio_sha256_simd",
        importpath = "github.com/minio/sha256-simd",
        sum = "h1:6kaan5IFmwTNynnKKpDHe6FWHohJOHhCPchzK49dzMM=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_mitchellh_cli",
        importpath = "github.com/mitchellh/cli",
        sum = "h1:iGBIsUe3+HZ/AD/Vd7DErOt5sU9fa8Uj7A2s1aggv1Y=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_mitchellh_colorstring",
        importpath = "github.com/mitchellh/colorstring",
        sum = "h1:62I3jR2EmQ4l5rM/4FEfDWcRD+abF5XlKShorW5LRoQ=",
        version = "v0.0.0-20190213212951-d06e56a500db",
    )
    go_repository(
        name = "com_github_mitchellh_go_homedir",
        importpath = "github.com/mitchellh/go-homedir",
        sum = "h1:vKb8ShqSby24Yrqr/yDYkuFz8d0WUjys40rvnGC8aR0=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_mitchellh_go_testing_interface",
        importpath = "github.com/mitchellh/go-testing-interface",
        sum = "h1:fzU/JVNcaqHQEcVFAKeR41fkiLdIPrefOvVG1VZ96U0=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_mitchellh_gox",
        importpath = "github.com/mitchellh/gox",
        sum = "h1:lfGJxY7ToLJQjHHwi0EX6uYBdK78egf954SQl13PQJc=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_mitchellh_iochan",
        importpath = "github.com/mitchellh/iochan",
        sum = "h1:C+X3KsSTLFVBr/tK1eYN/vs4rJcvsiLU338UhYPJWeY=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_mitchellh_mapstructure",
        importpath = "github.com/mitchellh/mapstructure",
        sum = "h1:CpVNEelQCZBooIPDn+AR3NpivK/TIKU8bDxdASFVQag=",
        version = "v1.4.1",
    )
    go_repository(
        name = "com_github_mitchellh_pointerstructure",
        importpath = "github.com/mitchellh/pointerstructure",
        sum = "h1:O+i9nHnXS3l/9Wu7r4NrEdwA2VFTicjUEN1uBnDo34A=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_mmcloughlin_addchain",
        importpath = "github.com/mmcloughlin/addchain",
        sum = "h1:SobOdjm2xLj1KkXN5/n0xTIWyZA2+s99UCY1iPfkHRY=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_mmcloughlin_profile",
        importpath = "github.com/mmcloughlin/profile",
        sum = "h1:jhDmAqPyebOsVDOCICJoINoLb/AnLBaUw58nFzxWS2w=",
        version = "v0.1.1",
    )
    go_repository(
        name = "com_github_moby_spdystream",
        importpath = "github.com/moby/spdystream",
        sum = "h1:cjW1zVyyoiM0T7b6UoySUFqzXMoqRckQtXwGPiBhOM8=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_modern_go_concurrent",
        importpath = "github.com/modern-go/concurrent",
        sum = "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
        version = "v0.0.0-20180306012644-bacd9c7ef1dd",
    )
    go_repository(
        name = "com_github_modern_go_reflect2",
        importpath = "github.com/modern-go/reflect2",
        sum = "h1:xBagoLtFs94CBntxluKeaWgTMpvLxC4ur3nMaC9Gz0M=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_mohae_deepcopy",
        importpath = "github.com/mohae/deepcopy",
        sum = "h1:RWengNIwukTxcDr9M+97sNutRR1RKhG96O6jWumTTnw=",
        version = "v0.0.0-20170929034955-c48cc78d4826",
    )
    go_repository(
        name = "com_github_mr_tron_base58",
        importpath = "github.com/mr-tron/base58",
        sum = "h1:T/HDJBh4ZCPbU39/+c3rRvE0uKBQlU27+QI8LJ4t64o=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_multiformats_go_base32",
        importpath = "github.com/multiformats/go-base32",
        sum = "h1:pVx9xoSPqEIQG8o+UbAe7DNi51oej1NtK+aGkbLYxPE=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_multiformats_go_base36",
        importpath = "github.com/multiformats/go-base36",
        sum = "h1:lFsAbNOGeKtuKozrtBsAkSVhv1p9D0/qedU9rQyccr0=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multiaddr",
        importpath = "github.com/multiformats/go-multiaddr",
        sum = "h1:BCBzs61E3AGHcYYTv8dqRH43ZfyrqM8RXVPT8t13tLQ=",
        version = "v0.13.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multiaddr_dns",
        importpath = "github.com/multiformats/go-multiaddr-dns",
        sum = "h1:P76EJ3qzBXpUXZ3twdCDx/kvagMsNo0LMFXpyms/zgU=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multiaddr_fmt",
        importpath = "github.com/multiformats/go-multiaddr-fmt",
        sum = "h1:WLEFClPycPkp4fnIzoFoV9FVd49/eQsuaL3/CWe167E=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multibase",
        importpath = "github.com/multiformats/go-multibase",
        sum = "h1:isdYCVLvksgWlMW9OZRYJEa9pZETFivncJHmHnnd87g=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multicodec",
        build_directives = [
            "gazelle:exclude gen.go",
        ],
        importpath = "github.com/multiformats/go-multicodec",
        sum = "h1:pb/dlPnzee/Sxv/j4PmkDRxCOi3hXTz3IbPKOXWJkmg=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_github_multiformats_go_multihash",
        importpath = "github.com/multiformats/go-multihash",
        sum = "h1:7Lyc8XfX/IY2jWb/gI7JP+o7JEq9hOa7BFvVU9RSh+U=",
        version = "v0.2.3",
    )
    go_repository(
        name = "com_github_multiformats_go_multistream",
        importpath = "github.com/multiformats/go-multistream",
        sum = "h1:5htLSLl7lvJk3xx3qT/8Zm9J4K8vEOf/QGkvOGQAyiE=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_github_multiformats_go_varint",
        importpath = "github.com/multiformats/go-varint",
        sum = "h1:sWSGR+f/eu5ABZA2ZpYKBILXTTs9JWpdEM/nEGOHFS8=",
        version = "v0.0.7",
    )
    go_repository(
        name = "com_github_munnerz_goautoneg",
        importpath = "github.com/munnerz/goautoneg",
        sum = "h1:C3w9PqII01/Oq1c1nUAm88MOHcQC9l5mIlSMApZMrHA=",
        version = "v0.0.0-20191010083416-a7dc8b61c822",
    )
    go_repository(
        name = "com_github_mwitkow_go_conntrack",
        importpath = "github.com/mwitkow/go-conntrack",
        sum = "h1:KUppIJq7/+SVif2QVs3tOP0zanoHgBEVAwHxUSIzRqU=",
        version = "v0.0.0-20190716064945-2f068394615f",
    )
    go_repository(
        name = "com_github_mxk_go_flowrate",
        importpath = "github.com/mxk/go-flowrate",
        sum = "h1:y5//uYreIhSUg3J1GEMiLbxo1LJaP8RfCpH6pymGZus=",
        version = "v0.0.0-20140419014527-cca7078d478f",
    )
    go_repository(
        name = "com_github_naoina_go_stringutil",
        importpath = "github.com/naoina/go-stringutil",
        sum = "h1:rCUeRUHjBjGTSHl0VC00jUPLz8/F9dDzYI70Hzifhks=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_naoina_toml",
        importpath = "github.com/naoina/toml",
        sum = "h1:shk/vn9oCoOTmwcouEdwIeOtOGA/ELRUw/GwvxwfT+0=",
        version = "v0.1.2-0.20170918210437-9fafd6967416",
    )
    go_repository(
        name = "com_github_nats_io_jwt",
        importpath = "github.com/nats-io/jwt",
        sum = "h1:+RB5hMpXUUA2dfxuhBTEkMOrYmM+gKIZYS1KjSostMI=",
        version = "v0.3.2",
    )
    go_repository(
        name = "com_github_nats_io_nats_go",
        importpath = "github.com/nats-io/nats.go",
        sum = "h1:ik3HbLhZ0YABLto7iX80pZLPw/6dx3T+++MZJwLnMrQ=",
        version = "v1.9.1",
    )
    go_repository(
        name = "com_github_nats_io_nats_server_v2",
        importpath = "github.com/nats-io/nats-server/v2",
        sum = "h1:i2Ly0B+1+rzNZHHWtD4ZwKi+OU5l+uQo1iDHZ2PmiIc=",
        version = "v2.1.2",
    )
    go_repository(
        name = "com_github_nats_io_nkeys",
        importpath = "github.com/nats-io/nkeys",
        sum = "h1:6JrEfig+HzTH85yxzhSVbjHRJv9cn0p6n3IngIcM5/k=",
        version = "v0.1.3",
    )
    go_repository(
        name = "com_github_nats_io_nuid",
        importpath = "github.com/nats-io/nuid",
        sum = "h1:5iA8DT8V7q8WK2EScv2padNa/rTESc1KdnPw4TC2paw=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_neelance_astrewrite",
        importpath = "github.com/neelance/astrewrite",
        sum = "h1:D6paGObi5Wud7xg83MaEFyjxQB1W5bz5d0IFppr+ymk=",
        version = "v0.0.0-20160511093645-99348263ae86",
    )
    go_repository(
        name = "com_github_neelance_sourcemap",
        importpath = "github.com/neelance/sourcemap",
        sum = "h1:eFXv9Nu1lGbrNbj619aWwZfVF5HBrm9Plte8aNptuTI=",
        version = "v0.0.0-20151028013722-8c68805598ab",
    )
    go_repository(
        name = "com_github_nxadm_tail",
        importpath = "github.com/nxadm/tail",
        sum = "h1:8feyoE3OzPrcshW5/MJ4sGESc5cqmGkGCWlco4l0bqY=",
        version = "v1.4.11",
    )
    go_repository(
        name = "com_github_oklog_oklog",
        importpath = "github.com/oklog/oklog",
        sum = "h1:wVfs8F+in6nTBMkA7CbRw+zZMIB7nNM825cM1wuzoTk=",
        version = "v0.3.2",
    )
    go_repository(
        name = "com_github_oklog_run",
        importpath = "github.com/oklog/run",
        sum = "h1:Ru7dDtJNOyC66gQ5dQmaCa0qIsAUFY3sFpK1Xk8igrw=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_olekukonko_tablewriter",
        importpath = "github.com/olekukonko/tablewriter",
        sum = "h1:P2Ga83D34wi1o9J6Wh1mRuqd4mF/x/lgBS7N7AbDhec=",
        version = "v0.0.5",
    )
    go_repository(
        name = "com_github_oneofone_xxhash",
        importpath = "github.com/OneOfOne/xxhash",
        sum = "h1:KMrpdQIwFcEqXDklaen+P1axHaj9BSKzvpUUfnHldSE=",
        version = "v1.2.2",
    )
    go_repository(
        name = "com_github_onsi_ginkgo",
        importpath = "github.com/onsi/ginkgo",
        sum = "h1:8xi0RTUf59SOSfEtZMvwTvXYMzG4gV23XVHOZiXNtnE=",
        version = "v1.16.5",
    )
    go_repository(
        name = "com_github_onsi_ginkgo_v2",
        importpath = "github.com/onsi/ginkgo/v2",
        sum = "h1:PE84V2mHqoT1sglvHc8ZdQtPcwmvvt29WLEEO3xmdZw=",
        version = "v2.20.0",
    )
    go_repository(
        name = "com_github_onsi_gomega",
        importpath = "github.com/onsi/gomega",
        sum = "h1:EUMJIKUjM8sKjYbtxQI9A4z2o+rruxnzNvpknOXie6k=",
        version = "v1.34.1",
    )
    go_repository(
        name = "com_github_op_go_logging",
        importpath = "github.com/op/go-logging",
        sum = "h1:lDH9UUVJtmYCjyT0CI4q8xvlXPxeZ0gYCVvWbmPlp88=",
        version = "v0.0.0-20160315200505-970db520ece7",
    )
    go_repository(
        name = "com_github_openconfig_gnmi",
        importpath = "github.com/openconfig/gnmi",
        sum = "h1:a380JP+B7xlMbEQOlha1buKhzBPXFqgFXplyWCEIGEY=",
        version = "v0.0.0-20190823184014-89b2bf29312c",
    )
    go_repository(
        name = "com_github_openconfig_reference",
        importpath = "github.com/openconfig/reference",
        sum = "h1:yHCGAHg2zMaW8olLrqEt3SAHGcEx2aJPEQWMRCyravY=",
        version = "v0.0.0-20190727015836-8dfd928c9696",
    )
    go_repository(
        name = "com_github_opencontainers_runtime_spec",
        importpath = "github.com/opencontainers/runtime-spec",
        sum = "h1:z97+pHb3uELt/yiAWD691HNHQIF07bE7dzrbT927iTk=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_opentracing_basictracer_go",
        importpath = "github.com/opentracing/basictracer-go",
        sum = "h1:YyUAhaEfjoWXclZVJ9sGoNct7j4TVk7lZWlQw5UXuoo=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_opentracing_contrib_go_observer",
        importpath = "github.com/opentracing-contrib/go-observer",
        sum = "h1:lM6RxxfUMrYL/f8bWEUqdXrANWtrL7Nndbm9iFN0DlU=",
        version = "v0.0.0-20170622124052-a52f23424492",
    )
    go_repository(
        name = "com_github_opentracing_opentracing_go",
        importpath = "github.com/opentracing/opentracing-go",
        sum = "h1:uEJPy/1a5RIPAJ0Ov+OIO8OxWu77jEv+1B0VhjKrZUs=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_openzipkin_contrib_zipkin_go_opentracing",
        importpath = "github.com/openzipkin-contrib/zipkin-go-opentracing",
        sum = "h1:ZCnq+JUrvXcDVhX/xRolRBZifmabN1HcS1wrPSvxhrU=",
        version = "v0.4.5",
    )
    go_repository(
        name = "com_github_openzipkin_zipkin_go",
        importpath = "github.com/openzipkin/zipkin-go",
        sum = "h1:nY8Hti+WKaP0cRsSeQ026wU03QsM762XBeCXBb9NAWI=",
        version = "v0.2.2",
    )
    go_repository(
        name = "com_github_otiai10_copy",
        importpath = "github.com/otiai10/copy",
        sum = "h1:HvG945u96iNadPoG2/Ja2+AUJeW5YuFQMixq9yirC+k=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_otiai10_curr",
        importpath = "github.com/otiai10/curr",
        sum = "h1:TJIWdbX0B+kpNagQrjgq8bCMrbhiuX73M2XwgtDMoOI=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_otiai10_mint",
        importpath = "github.com/otiai10/mint",
        sum = "h1:BCmzIS3n71sGfHB5NMNDB3lHYPz8fWSkCAErHed//qc=",
        version = "v1.3.1",
    )
    go_repository(
        name = "com_github_pact_foundation_pact_go",
        importpath = "github.com/pact-foundation/pact-go",
        sum = "h1:OYkFijGHoZAYbOIb1LWXrwKQbMMRUv1oQ89blD2Mh2Q=",
        version = "v1.0.4",
    )
    go_repository(
        name = "com_github_pascaldekloe_goe",
        importpath = "github.com/pascaldekloe/goe",
        sum = "h1:Lgl0gzECD8GnQ5QCWA8o6BtfL6mDH5rQgM4/fX3avOs=",
        version = "v0.0.0-20180627143212-57f6aae5913c",
    )
    go_repository(
        name = "com_github_patrickmn_go_cache",
        importpath = "github.com/patrickmn/go-cache",
        sum = "h1:HRMgzkcYKYpi3C8ajMPV8OFXaaRUnok+kx1WdO15EQc=",
        version = "v2.1.0+incompatible",
    )
    go_repository(
        name = "com_github_paulbellamy_ratecounter",
        importpath = "github.com/paulbellamy/ratecounter",
        sum = "h1:2L/RhJq+HA8gBQImDXtLPrDXK5qAj6ozWVK/zFXVJGs=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_pbnjay_memory",
        importpath = "github.com/pbnjay/memory",
        sum = "h1:onHthvaw9LFnH4t2DcNVpwGmV9E1BkGknEliJkfwQj0=",
        version = "v0.0.0-20210728143218-7b4eea64cf58",
    )
    go_repository(
        name = "com_github_pborman_uuid",
        importpath = "github.com/pborman/uuid",
        sum = "h1:+ZZIw58t/ozdjRaXh/3awHfmWRbzYxJoAdNJxe/3pvw=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_pelletier_go_toml",
        importpath = "github.com/pelletier/go-toml",
        sum = "h1:6P7XZEBu/ZWizC/liUX4UYm4nEAACofmSkOzY39RBxM=",
        version = "v1.0.1-0.20170904195809-1d6b12b7cb29",
    )
    go_repository(
        name = "com_github_pelletier_go_toml_v2",
        importpath = "github.com/pelletier/go-toml/v2",
        sum = "h1:ipoSadvV8oGUjnUbMub59IDPPwfxF694nG/jwbMiyQg=",
        version = "v2.0.5",
    )
    go_repository(
        name = "com_github_performancecopilot_speed",
        importpath = "github.com/performancecopilot/speed",
        sum = "h1:2WnRzIquHa5QxaJKShDkLM+sc0JPuwhXzK8OYOyt3Vg=",
        version = "v3.0.0+incompatible",
    )
    go_repository(
        name = "com_github_peterbourgon_diskv",
        importpath = "github.com/peterbourgon/diskv",
        sum = "h1:UBdAOUP5p4RWqPBg048CAvpKN+vxiaj6gdUUzhl4XmI=",
        version = "v2.0.1+incompatible",
    )
    go_repository(
        name = "com_github_peterh_liner",
        importpath = "github.com/peterh/liner",
        sum = "h1:w/UPXyl5GfahFxcTOz2j9wCIHNI+pUPr2laqpojKNCg=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_pierrec_lz4",
        importpath = "github.com/pierrec/lz4",
        sum = "h1:mFe7ttWaflA46Mhqh+jUfjp2qTbPYxLB2/OyBppH9dg=",
        version = "v2.4.1+incompatible",
    )
    go_repository(
        name = "com_github_pingcap_errors",
        importpath = "github.com/pingcap/errors",
        sum = "h1:lFuQV/oaUMGcD2tqt+01ROSmJs75VG1ToEOkZIZ4nE4=",
        version = "v0.11.4",
    )
    go_repository(
        name = "com_github_pion_datachannel",
        importpath = "github.com/pion/datachannel",
        sum = "h1:ph1P1NsGkazkjrvyMfhRBUAWMxugJjq2HfQifaOoSNo=",
        version = "v1.5.8",
    )
    go_repository(
        name = "com_github_pion_dtls_v2",
        importpath = "github.com/pion/dtls/v2",
        sum = "h1:KP7H5/c1EiVAAKUmXyCzPiQe5+bCJrpOeKg/L05dunk=",
        version = "v2.2.12",
    )
    go_repository(
        name = "com_github_pion_ice_v2",
        importpath = "github.com/pion/ice/v2",
        sum = "h1:Ic1ppYCj4tUOcPAp76U6F3fVrlSw8A9JtRXLqw6BbUM=",
        version = "v2.3.34",
    )
    go_repository(
        name = "com_github_pion_interceptor",
        importpath = "github.com/pion/interceptor",
        sum = "h1:au5rlVHsgmxNi+v/mjOPazbW1SHzfx7/hYOEYQnUcxA=",
        version = "v0.1.30",
    )
    go_repository(
        name = "com_github_pion_logging",
        importpath = "github.com/pion/logging",
        sum = "h1:M9+AIj/+pxNsDfAT64+MAVgJO0rsyLnoJKCqf//DoeY=",
        version = "v0.2.2",
    )
    go_repository(
        name = "com_github_pion_mdns",
        importpath = "github.com/pion/mdns",
        sum = "h1:CiMYlY+O0azojWDmxdNr7ADGrnZ+V6Ilfner+6mSVK8=",
        version = "v0.0.12",
    )
    go_repository(
        name = "com_github_pion_randutil",
        importpath = "github.com/pion/randutil",
        sum = "h1:CFG1UdESneORglEsnimhUjf33Rwjubwj6xfiOXBa3mA=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_pion_rtcp",
        importpath = "github.com/pion/rtcp",
        sum = "h1:KCkGV3vJ+4DAJmvP0vaQShsb0xkRfWkO540Gy102KyE=",
        version = "v1.2.14",
    )
    go_repository(
        name = "com_github_pion_rtp",
        importpath = "github.com/pion/rtp",
        sum = "h1:E2HX740TZKaqdcPmf4pw6ZZuG8u5RlMMt+l3dxeu6Wk=",
        version = "v1.8.9",
    )
    go_repository(
        name = "com_github_pion_sctp",
        importpath = "github.com/pion/sctp",
        sum = "h1:dSE4wX6uTJBcNm8+YlMg7lw1wqyKHggsP5uKbdj+NZw=",
        version = "v1.8.33",
    )
    go_repository(
        name = "com_github_pion_sdp_v3",
        importpath = "github.com/pion/sdp/v3",
        sum = "h1:pX++dCHoHUwq43kuwf3PyJfHlwIj4hXA7Vrifiq0IJY=",
        version = "v3.0.9",
    )
    go_repository(
        name = "com_github_pion_srtp_v2",
        importpath = "github.com/pion/srtp/v2",
        sum = "h1:HNNny4s+OUmG280ETrCdgFndp4ufx3/uy85EawYEhTk=",
        version = "v2.0.20",
    )
    go_repository(
        name = "com_github_pion_stun",
        importpath = "github.com/pion/stun",
        sum = "h1:8lp6YejULeHBF8NmV8e2787BogQhduZugh5PdhDyyN4=",
        version = "v0.6.1",
    )
    go_repository(
        name = "com_github_pion_transport_v2",
        importpath = "github.com/pion/transport/v2",
        sum = "h1:ucLBLE8nuxiHfvkFKnkDQRYWYfp8ejf4YBOPfaQpw6Q=",
        version = "v2.2.10",
    )
    go_repository(
        name = "com_github_pion_transport_v3",
        importpath = "github.com/pion/transport/v3",
        sum = "h1:iRbMH05BzSNwhILHoBoAPxoB9xQgOaJk+591KC9P1o0=",
        version = "v3.0.7",
    )
    go_repository(
        name = "com_github_pion_turn_v2",
        importpath = "github.com/pion/turn/v2",
        sum = "h1:Xr2niVsiPTB0FPtt+yAWKFUkU1eotQbGgpTIld4x1Gc=",
        version = "v2.1.6",
    )
    go_repository(
        name = "com_github_pion_webrtc_v3",
        importpath = "github.com/pion/webrtc/v3",
        sum = "h1:Rf4u6n6U5t5sUxhYPQk/samzU/oDv7jk6BA5hyO2F9I=",
        version = "v3.3.0",
    )
    go_repository(
        name = "com_github_pkg_diff",
        importpath = "github.com/pkg/diff",
        sum = "h1:aoZm08cpOy4WuID//EZDgcC4zIxODThtZNPirFr42+A=",
        version = "v0.0.0-20210226163009-20ebb0f2a09e",
    )
    go_repository(
        name = "com_github_pkg_errors",
        importpath = "github.com/pkg/errors",
        sum = "h1:FEBLx1zS214owpjy7qsBeixbURkuhQAwrK5UwLGTwt4=",
        version = "v0.9.1",
    )
    go_repository(
        name = "com_github_pkg_profile",
        importpath = "github.com/pkg/profile",
        sum = "h1:hnbDkaNWPCLMO9wGLdBFTIZvzDrDfBM2072E1S9gJkA=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_github_pkg_sftp",
        importpath = "github.com/pkg/sftp",
        sum = "h1:I2qBYMChEhIjOgazfJmV3/mZM256btk6wkCDRmW7JYs=",
        version = "v1.13.1",
    )
    go_repository(
        name = "com_github_pmezard_go_difflib",
        importpath = "github.com/pmezard/go-difflib",
        sum = "h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_posener_complete",
        importpath = "github.com/posener/complete",
        sum = "h1:ccV59UEOTzVDnDUEFdT95ZzHVZ+5+158q8+SJb2QV5w=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_prashantv_gostub",
        importpath = "github.com/prashantv/gostub",
        sum = "h1:BTyx3RfQjRHnUWaGF9oQos79AlQ5k8WNktv7VGvVH4g=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_prometheus_client_golang",
        importpath = "github.com/prometheus/client_golang",
        sum = "h1:jBzTZ7B099Rg24tny+qngoynol8LtVYlA2bqx3vEloI=",
        version = "v1.20.0",
    )
    go_repository(
        name = "com_github_prometheus_client_model",
        importpath = "github.com/prometheus/client_model",
        sum = "h1:ZKSh/rekM+n3CeS952MLRAdFwIKqeY8b62p8ais2e9E=",
        version = "v0.6.1",
    )
    go_repository(
        name = "com_github_prometheus_common",
        importpath = "github.com/prometheus/common",
        sum = "h1:KEi6DK7lXW/m7Ig5i47x0vRzuBsHuvJdi5ee6Y3G1dc=",
        version = "v0.55.0",
    )
    go_repository(
        name = "com_github_prometheus_procfs",
        importpath = "github.com/prometheus/procfs",
        sum = "h1:YagwOFzUgYfKKHX6Dr+sHT7km/hxC76UB0learggepc=",
        version = "v0.15.1",
    )
    go_repository(
        name = "com_github_prometheus_prom2json",
        importpath = "github.com/prometheus/prom2json",
        sum = "h1:BlqrtbT9lLH3ZsOVhXPsHzFrApCTKRifB7gjJuypu6Y=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_protolambda_bls12_381_util",
        importpath = "github.com/protolambda/bls12-381-util",
        sum = "h1:cZC+usqsYgHtlBaGulVnZ1hfKAi8iWtujBnRLQE698c=",
        version = "v0.0.0-20220416220906-d8552aa452c7",
    )
    go_repository(
        name = "com_github_prysmaticlabs_fastssz",
        importpath = "github.com/prysmaticlabs/fastssz",
        sum = "h1:xuVAdtz5ShYblG2sPyb4gw01DF8InbOI/kBCQjk7NiM=",
        version = "v0.0.0-20241008181541-518c4ce73516",
    )
    go_repository(
        name = "com_github_prysmaticlabs_go_bitfield",
        importpath = "github.com/prysmaticlabs/go-bitfield",
        sum = "h1:ATgOe+abbzfx9kCPeXIW4fiWyDdxlwHw07j8UGhdTd4=",
        version = "v0.0.0-20240328144219-a1caa50c3a1e",
    )
    go_repository(
        name = "com_github_prysmaticlabs_gohashtree",
        importpath = "github.com/prysmaticlabs/gohashtree",
        sum = "h1:VK7thFOnhxAZ/5aolr5Os4beiubuD08WiuiHyRqgwks=",
        version = "v0.0.4-beta.0.20240624100937-73632381301b",
    )
    go_repository(
        name = "com_github_prysmaticlabs_prombbolt",
        importpath = "github.com/prysmaticlabs/prombbolt",
        sum = "h1:9PHRCuO/VN0s9k+RmLykho7AjDxblNYI5bYKed16NPU=",
        version = "v0.0.0-20210126082820-9b7adba6db7c",
    )
    go_repository(
        name = "com_github_prysmaticlabs_protoc_gen_go_cast",
        importpath = "github.com/prysmaticlabs/protoc-gen-go-cast",
        sum = "h1:q9wE0ZZRdTUAAeyFP/w0SwBEnCqlVy2+on6X2/e+eAU=",
        version = "v0.0.0-20230228205207-28762a7b9294",
    )
    go_repository(
        name = "com_github_quic_go_qpack",
        build_directives = [
            "gazelle:exclude tools.go",
        ],
        importpath = "github.com/quic-go/qpack",
        sum = "h1:giqksBPnT/HDtZ6VhtFKgoLOWmlyo9Ei6u9PqzIMbhI=",
        version = "v0.5.1",
    )
    go_repository(
        name = "com_github_quic_go_quic_go",
        build_directives = [
            "gazelle:exclude mockgen.go",
            "gazelle:exclude tools.go",
        ],
        importpath = "github.com/quic-go/quic-go",
        sum = "h1:wsKXZPeGWpMpCGSWqOcqpW2wZYic/8T3aqiOID0/KWE=",
        version = "v0.48.2",
    )
    go_repository(
        name = "com_github_quic_go_webtransport_go",
        importpath = "github.com/quic-go/webtransport-go",
        sum = "h1:4WFk6u3sOT6pLa1kQ50ZVdm8BQFgJNA117cepZxtLIg=",
        version = "v0.8.1-0.20241018022711-4ac2c9250e66",
    )
    go_repository(
        name = "com_github_r3labs_sse_v2",
        importpath = "github.com/r3labs/sse/v2",
        sum = "h1:hFEkLLFY4LDifoHdiCN/LlGBAdVJYsANaLqNYa1l/v0=",
        version = "v2.10.0",
    )
    go_repository(
        name = "com_github_raulk_go_watchdog",
        importpath = "github.com/raulk/go-watchdog",
        sum = "h1:oUmdlHxdkXRJlwfG0O9omj8ukerm8MEQavSiDTEtBsk=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_rcrowley_go_metrics",
        importpath = "github.com/rcrowley/go-metrics",
        sum = "h1:dY6ETXrvDG7Sa4vE8ZQG4yqWg6UnOcbqTAahkV813vQ=",
        version = "v0.0.0-20190826022208-cac0b30c2563",
    )
    go_repository(
        name = "com_github_rivo_tview",
        importpath = "github.com/rivo/tview",
        sum = "h1:7UMY2qN9VlcY+x9jlhpYe5Bf1zrdhvmfZyLMk2u65BM=",
        version = "v0.0.0-20231126152417-33a1d271f2b6",
    )
    go_repository(
        name = "com_github_rivo_uniseg",
        build_directives = [
            "gazelle:exclude gen_breaktest.go",
            "gazelle:exclude gen_properties.go",
        ],
        importpath = "github.com/rivo/uniseg",
        sum = "h1:8TfxU8dW6PdqD27gjM8MVNuicgxIjxpm4K7x4jp8sis=",
        version = "v0.4.4",
    )
    go_repository(
        name = "com_github_rogpeppe_fastuuid",
        importpath = "github.com/rogpeppe/fastuuid",
        sum = "h1:gu+uRPtBe88sKxUCEXRoeCvVG90TJmwhiqRpvdhQFng=",
        version = "v0.0.0-20150106093220-6724a57986af",
    )
    go_repository(
        name = "com_github_rogpeppe_go_internal",
        importpath = "github.com/rogpeppe/go-internal",
        sum = "h1:exVL4IDcn6na9z1rAb56Vxr+CgyK3nn3O+epU5NdKM8=",
        version = "v1.12.0",
    )
    go_repository(
        name = "com_github_rs_cors",
        importpath = "github.com/rs/cors",
        sum = "h1:+88SsELBHx5r+hZ8TCkggzSstaWNbDvThkVK8H6f9ik=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_github_russross_blackfriday",
        importpath = "github.com/russross/blackfriday",
        sum = "h1:HyvC0ARfnZBqnXwABFeSZHpKvJHJJfPz81GNueLj0oo=",
        version = "v1.5.2",
    )
    go_repository(
        name = "com_github_russross_blackfriday_v2",
        importpath = "github.com/russross/blackfriday/v2",
        sum = "h1:JIOH55/0cWyOuilr9/qlrm0BSXldqnqwMsf35Ld67mk=",
        version = "v2.1.0",
    )
    go_repository(
        name = "com_github_ryanuber_columnize",
        importpath = "github.com/ryanuber/columnize",
        sum = "h1:UFr9zpz4xgTnIE5yIMtWAMngCdZ9p/+q6lTbgelo80M=",
        version = "v0.0.0-20160712163229-9b3edd62028f",
    )
    go_repository(
        name = "com_github_samuel_go_zookeeper",
        importpath = "github.com/samuel/go-zookeeper",
        sum = "h1:p3Vo3i64TCLY7gIfzeQaUJ+kppEO5WQG3cL8iE8tGHU=",
        version = "v0.0.0-20190923202752-2cc03de413da",
    )
    go_repository(
        name = "com_github_satori_go_uuid",
        importpath = "github.com/satori/go.uuid",
        sum = "h1:0uYX9dsZ2yD7q2RtLRtPSdGDWzjeM3TbMJP9utgA0ww=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_schollz_closestmatch",
        importpath = "github.com/schollz/closestmatch",
        sum = "h1:Uel2GXEpJqOWBrlyI+oY9LTiyyjYS17cCYRqP13/SHk=",
        version = "v2.1.0+incompatible",
    )
    go_repository(
        name = "com_github_schollz_progressbar_v3",
        importpath = "github.com/schollz/progressbar/v3",
        sum = "h1:nMinx+JaEm/zJz4cEyClQeAw5rsYSB5th3xv+5lV6Vg=",
        version = "v3.3.4",
    )
    go_repository(
        name = "com_github_sclevine_agouti",
        importpath = "github.com/sclevine/agouti",
        sum = "h1:8IBJS6PWz3uTlMP3YBIR5f+KAldcGuOeFkFbUWfBgK4=",
        version = "v3.0.0+incompatible",
    )
    go_repository(
        name = "com_github_sean_seed",
        importpath = "github.com/sean-/seed",
        sum = "h1:nn5Wsu0esKSJiIVhscUtVbo7ada43DJhG55ua/hjS5I=",
        version = "v0.0.0-20170313163322-e2103e2c3529",
    )
    go_repository(
        name = "com_github_sergi_go_diff",
        importpath = "github.com/sergi/go-diff",
        sum = "h1:Kpca3qRNrduNnOQeazBd0ysaKrUJiIuISHxogkT9RPQ=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_shirou_gopsutil",
        importpath = "github.com/shirou/gopsutil",
        sum = "h1:+1+c1VGhc88SSonWP6foOcLhvnKlUeu/erjjvaPEYiI=",
        version = "v3.21.11+incompatible",
    )
    go_repository(
        name = "com_github_shopify_goreferrer",
        importpath = "github.com/Shopify/goreferrer",
        sum = "h1:KkH3I3sJuOLP3TjA/dfr4NAY8bghDwnXiU7cTKxQqo0=",
        version = "v0.0.0-20220729165902-8cddb4f5de06",
    )
    go_repository(
        name = "com_github_shopify_sarama",
        importpath = "github.com/Shopify/sarama",
        sum = "h1:3jnfWKD7gVwbB1KSy/lE0szA9duPuSFLViK0o/d3DgA=",
        version = "v1.26.1",
    )
    go_repository(
        name = "com_github_shopify_toxiproxy",
        importpath = "github.com/Shopify/toxiproxy",
        sum = "h1:TKdv8HiTLgE5wdJuEML90aBgNWsokNbMijUGhmcoBJc=",
        version = "v2.1.4+incompatible",
    )
    go_repository(
        name = "com_github_shurcool_component",
        importpath = "github.com/shurcooL/component",
        sum = "h1:Fth6mevc5rX7glNLpbAMJnqKlfIkcTjZCSHEeqvKbcI=",
        version = "v0.0.0-20170202220835-f88ec8f54cc4",
    )
    go_repository(
        name = "com_github_shurcool_events",
        importpath = "github.com/shurcooL/events",
        sum = "h1:vabduItPAIz9px5iryD5peyx7O3Ya8TBThapgXim98o=",
        version = "v0.0.0-20181021180414-410e4ca65f48",
    )
    go_repository(
        name = "com_github_shurcool_github_flavored_markdown",
        importpath = "github.com/shurcooL/github_flavored_markdown",
        sum = "h1:qb9IthCFBmROJ6YBS31BEMeSYjOscSiG+EO+JVNTz64=",
        version = "v0.0.0-20181002035957-2122de532470",
    )
    go_repository(
        name = "com_github_shurcool_go",
        importpath = "github.com/shurcooL/go",
        sum = "h1:MZM7FHLqUHYI0Y/mQAt3d2aYa0SiNms/hFqC9qJYolM=",
        version = "v0.0.0-20180423040247-9e1955d9fb6e",
    )
    go_repository(
        name = "com_github_shurcool_go_goon",
        importpath = "github.com/shurcooL/go-goon",
        sum = "h1:llrF3Fs4018ePo4+G/HV/uQUqEI1HMDjCeOf2V6puPc=",
        version = "v0.0.0-20170922171312-37c2f522c041",
    )
    go_repository(
        name = "com_github_shurcool_gofontwoff",
        importpath = "github.com/shurcooL/gofontwoff",
        sum = "h1:Yoy/IzG4lULT6qZg62sVC+qyBL8DQkmD2zv6i7OImrc=",
        version = "v0.0.0-20180329035133-29b52fc0a18d",
    )
    go_repository(
        name = "com_github_shurcool_gopherjslib",
        importpath = "github.com/shurcooL/gopherjslib",
        sum = "h1:UOk+nlt1BJtTcH15CT7iNO7YVWTfTv/DNwEAQHLIaDQ=",
        version = "v0.0.0-20160914041154-feb6d3990c2c",
    )
    go_repository(
        name = "com_github_shurcool_highlight_diff",
        importpath = "github.com/shurcooL/highlight_diff",
        sum = "h1:vYEG87HxbU6dXj5npkeulCS96Dtz5xg3jcfCgpcvbIw=",
        version = "v0.0.0-20170515013008-09bb4053de1b",
    )
    go_repository(
        name = "com_github_shurcool_highlight_go",
        importpath = "github.com/shurcooL/highlight_go",
        sum = "h1:7pDq9pAMCQgRohFmd25X8hIH8VxmT3TaDm+r9LHxgBk=",
        version = "v0.0.0-20181028180052-98c3abbbae20",
    )
    go_repository(
        name = "com_github_shurcool_home",
        importpath = "github.com/shurcooL/home",
        sum = "h1:MPblCbqA5+z6XARjScMfz1TqtJC7TuTRj0U9VqIBs6k=",
        version = "v0.0.0-20181020052607-80b7ffcb30f9",
    )
    go_repository(
        name = "com_github_shurcool_htmlg",
        importpath = "github.com/shurcooL/htmlg",
        sum = "h1:crYRwvwjdVh1biHzzciFHe8DrZcYrVcZFlJtykhRctg=",
        version = "v0.0.0-20170918183704-d01228ac9e50",
    )
    go_repository(
        name = "com_github_shurcool_httperror",
        importpath = "github.com/shurcooL/httperror",
        sum = "h1:eHRtZoIi6n9Wo1uR+RU44C247msLWwyA89hVKwRLkMk=",
        version = "v0.0.0-20170206035902-86b7830d14cc",
    )
    go_repository(
        name = "com_github_shurcool_httpfs",
        importpath = "github.com/shurcooL/httpfs",
        sum = "h1:SWV2fHctRpRrp49VXJ6UZja7gU9QLHwRpIPBN89SKEo=",
        version = "v0.0.0-20171119174359-809beceb2371",
    )
    go_repository(
        name = "com_github_shurcool_httpgzip",
        importpath = "github.com/shurcooL/httpgzip",
        sum = "h1:fxoFD0in0/CBzXoyNhMTjvBZYW6ilSnTw7N7y/8vkmM=",
        version = "v0.0.0-20180522190206-b1c53ac65af9",
    )
    go_repository(
        name = "com_github_shurcool_issues",
        importpath = "github.com/shurcooL/issues",
        sum = "h1:T4wuULTrzCKMFlg3HmKHgXAF8oStFb/+lOIupLV2v+o=",
        version = "v0.0.0-20181008053335-6292fdc1e191",
    )
    go_repository(
        name = "com_github_shurcool_issuesapp",
        importpath = "github.com/shurcooL/issuesapp",
        sum = "h1:Y+TeIabU8sJD10Qwd/zMty2/LEaT9GNDaA6nyZf+jgo=",
        version = "v0.0.0-20180602232740-048589ce2241",
    )
    go_repository(
        name = "com_github_shurcool_notifications",
        importpath = "github.com/shurcooL/notifications",
        sum = "h1:TQVQrsyNaimGwF7bIhzoVC9QkKm4KsWd8cECGzFx8gI=",
        version = "v0.0.0-20181007000457-627ab5aea122",
    )
    go_repository(
        name = "com_github_shurcool_octicon",
        importpath = "github.com/shurcooL/octicon",
        sum = "h1:bu666BQci+y4S0tVRVjsHUeRon6vUXmsGBwdowgMrg4=",
        version = "v0.0.0-20181028054416-fa4f57f9efb2",
    )
    go_repository(
        name = "com_github_shurcool_reactions",
        importpath = "github.com/shurcooL/reactions",
        sum = "h1:LneqU9PHDsg/AkPDU3AkqMxnMYL+imaqkpflHu73us8=",
        version = "v0.0.0-20181006231557-f2e0b4ca5b82",
    )
    go_repository(
        name = "com_github_shurcool_sanitized_anchor_name",
        importpath = "github.com/shurcooL/sanitized_anchor_name",
        sum = "h1:PdmoCO6wvbs+7yrJyMORt4/BmY5IYyJwS/kOiWx8mHo=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_shurcool_users",
        importpath = "github.com/shurcooL/users",
        sum = "h1:YGaxtkYjb8mnTvtufv2LKLwCQu2/C7qFB7UtrOlTWOY=",
        version = "v0.0.0-20180125191416-49c67e49c537",
    )
    go_repository(
        name = "com_github_shurcool_webdavfs",
        importpath = "github.com/shurcooL/webdavfs",
        sum = "h1:JtcyT0rk/9PKOdnKQzuDR+FSjh7SGtJwpgVpfZBRKlQ=",
        version = "v0.0.0-20170829043945-18c3829fa133",
    )
    go_repository(
        name = "com_github_sirupsen_logrus",
        importpath = "github.com/sirupsen/logrus",
        sum = "h1:trlNQbNUG3OdDrDil03MCb1H2o9nJ1x4/5LYw7byDE0=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_github_smartystreets_assertions",
        importpath = "github.com/smartystreets/assertions",
        sum = "h1:zE9ykElWQ6/NYmHa3jpm/yHnI4xSofP+UP6SpjHcSeM=",
        version = "v0.0.0-20180927180507-b2de0cb4f26d",
    )
    go_repository(
        name = "com_github_smartystreets_goconvey",
        importpath = "github.com/smartystreets/goconvey",
        sum = "h1:fv0U8FUIMPNf1L9lnHLvLhgicrIVChEkdzIKYqbNC9s=",
        version = "v1.6.4",
    )
    go_repository(
        name = "com_github_soheilhy_cmux",
        importpath = "github.com/soheilhy/cmux",
        sum = "h1:0HKaf1o97UwFjHH9o5XsHUOF+tqmdA7KEzXLpiyaw0E=",
        version = "v0.1.4",
    )
    go_repository(
        name = "com_github_sony_gobreaker",
        importpath = "github.com/sony/gobreaker",
        sum = "h1:oMnRNZXX5j85zso6xCPRNPtmAycat+WcoKbklScLDgQ=",
        version = "v0.4.1",
    )
    go_repository(
        name = "com_github_sourcegraph_annotate",
        importpath = "github.com/sourcegraph/annotate",
        sum = "h1:yKm7XZV6j9Ev6lojP2XaIshpT4ymkqhMeSghO5Ps00E=",
        version = "v0.0.0-20160123013949-f4cad6c6324d",
    )
    go_repository(
        name = "com_github_sourcegraph_syntaxhighlight",
        importpath = "github.com/sourcegraph/syntaxhighlight",
        sum = "h1:qpG93cPwA5f7s/ZPBJnGOYQNK/vKsaDaseuKT5Asee8=",
        version = "v0.0.0-20170531221838-bd320f5d308e",
    )
    go_repository(
        name = "com_github_spaolacci_murmur3",
        importpath = "github.com/spaolacci/murmur3",
        sum = "h1:7c1g84S4BPRrfL5Xrdp6fOJ206sU9y293DDHaoy0bLI=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_spf13_afero",
        importpath = "github.com/spf13/afero",
        sum = "h1:EaGW2JJh15aKOejeuJ+wpFSHnbd7GE6Wvp3TsNhb6LY=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_github_spf13_cast",
        importpath = "github.com/spf13/cast",
        sum = "h1:0Rhw4d6C8J9VPu6cjZLIhZ8+aAOHcDvGeKn+cq5Aq3k=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_spf13_cobra",
        importpath = "github.com/spf13/cobra",
        sum = "h1:X+jTBEBqF0bHN+9cSMgmfuvv2VHJ9ezmFNf9Y/XstYU=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_github_spf13_jwalterweatherman",
        importpath = "github.com/spf13/jwalterweatherman",
        sum = "h1:zBoLErXXAvWnNsu+pWkRYl6Cx1KXmIfAVsIuYkPN6aY=",
        version = "v0.0.0-20170901151539-12bd96e66386",
    )
    go_repository(
        name = "com_github_spf13_pflag",
        importpath = "github.com/spf13/pflag",
        sum = "h1:iy+VFUOCP1a+8yFto/drg2CJ5u0yRoB7fZw3DKv/JXA=",
        version = "v1.0.5",
    )
    go_repository(
        name = "com_github_spf13_viper",
        importpath = "github.com/spf13/viper",
        sum = "h1:RUA/ghS2i64rlnn4ydTfblY8Og8QzcPtCcHvgMn+w/I=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_stackexchange_wmi",
        importpath = "github.com/StackExchange/wmi",
        sum = "h1:VIkavFPXSjcnS+O8yTq7NI32k0R5Aj+v39y29VYDOSA=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_status_im_keycard_go",
        importpath = "github.com/status-im/keycard-go",
        sum = "h1:QDLFswOQu1r5jsycloeQh3bVU8n/NatHHaZobtDnDzA=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_streadway_amqp",
        importpath = "github.com/streadway/amqp",
        sum = "h1:WhxRHzgeVGETMlmVfqhRn8RIeeNoPr2Czh33I4Zdccw=",
        version = "v0.0.0-20190827072141-edfb9018d271",
    )
    go_repository(
        name = "com_github_streadway_handy",
        importpath = "github.com/streadway/handy",
        sum = "h1:AhmOdSHeswKHBjhsLs/7+1voOxT+LLrSk/Nxvk35fug=",
        version = "v0.0.0-20190108123426-d5acb3125c2a",
    )
    go_repository(
        name = "com_github_stretchr_objx",
        importpath = "github.com/stretchr/objx",
        sum = "h1:xuMeJ0Sdp5ZMRXx/aWO6RZxdr3beISkG5/G/aIRr3pY=",
        version = "v0.5.2",
    )
    go_repository(
        name = "com_github_stretchr_testify",
        importpath = "github.com/stretchr/testify",
        sum = "h1:HtqpIVDClZ4nwg75+f6Lvsy/wHu+3BoSGCbBAcpTsTg=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_github_syndtr_goleveldb",
        importpath = "github.com/syndtr/goleveldb",
        sum = "h1:epCh84lMvA70Z7CTTCmYQn2CKbY8j86K7/FAIr141uY=",
        version = "v1.0.1-0.20210819022825-2ae1ddf74ef7",
    )
    go_repository(
        name = "com_github_tarm_serial",
        importpath = "github.com/tarm/serial",
        sum = "h1:UyzmZLoiDWMRywV4DUYb9Fbt8uiOSooupjTq10vpvnU=",
        version = "v0.0.0-20180830185346-98f6abe2eb07",
    )
    go_repository(
        name = "com_github_tdewolff_minify_v2",
        importpath = "github.com/tdewolff/minify/v2",
        sum = "h1:kejsHQMM17n6/gwdw53qsi6lg0TGddZADVyQOz1KMdE=",
        version = "v2.12.4",
    )
    go_repository(
        name = "com_github_tdewolff_parse_v2",
        importpath = "github.com/tdewolff/parse/v2",
        sum = "h1:KCkDvNUMof10e3QExio9OPZJT8SbdKojLBumw8YZycQ=",
        version = "v2.6.4",
    )
    go_repository(
        name = "com_github_templexxx_cpufeat",
        importpath = "github.com/templexxx/cpufeat",
        sum = "h1:89CEmDvlq/F7SJEOqkIdNDGJXrQIhuIx9D2DBXjavSU=",
        version = "v0.0.0-20180724012125-cef66df7f161",
    )
    go_repository(
        name = "com_github_templexxx_xor",
        importpath = "github.com/templexxx/xor",
        sum = "h1:fj5tQ8acgNUr6O8LEplsxDhUIe2573iLkJc+PqnzZTI=",
        version = "v0.0.0-20191217153810-f85b25db303b",
    )
    go_repository(
        name = "com_github_tenntenn_modver",
        importpath = "github.com/tenntenn/modver",
        sum = "h1:2klLppGhDgzJrScMpkj9Ujy3rXPUspSjAcev9tSEBgA=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_tenntenn_text_transform",
        importpath = "github.com/tenntenn/text/transform",
        sum = "h1:f+jULpRQGxTSkNYKJ51yaw6ChIqO+Je8UqsTKN/cDag=",
        version = "v0.0.0-20200319021203-7eef512accb3",
    )
    go_repository(
        name = "com_github_thomaso_mirodin_intmath",
        importpath = "github.com/thomaso-mirodin/intmath",
        sum = "h1:cR8/SYRgyQCt5cNCMniB/ZScMkhI9nk8U5C7SbISXjo=",
        version = "v0.0.0-20160323211736-5dc6d854e46e",
    )
    go_repository(
        name = "com_github_tjfoc_gmsm",
        importpath = "github.com/tjfoc/gmsm",
        sum = "h1:i7c6Za/IlgBvnGxYpfD7L3TGuaS+v6oGcgq+J9/ecEA=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_tklauser_go_sysconf",
        importpath = "github.com/tklauser/go-sysconf",
        sum = "h1:GBUpcahXSpR2xN01jhkNAbTLRk2Yzgggk8IM08lq3r4=",
        version = "v0.3.13",
    )
    go_repository(
        name = "com_github_tklauser_numcpus",
        importpath = "github.com/tklauser/numcpus",
        sum = "h1:yjuerZP127QG9m5Zh/mSO4wqurYil27tHrqwRoRjpr4=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_github_tmc_grpc_websocket_proxy",
        importpath = "github.com/tmc/grpc-websocket-proxy",
        sum = "h1:ndzgwNDnKIqyCvHTXaCqh9KlOWKvBry6nuXMJmonVsE=",
        version = "v0.0.0-20170815181823-89b8d40f7ca8",
    )
    go_repository(
        name = "com_github_trailofbits_go_mutexasserts",
        importpath = "github.com/trailofbits/go-mutexasserts",
        sum = "h1:+LynomhWB+14Plp/bOONEAZCtvCZk4leRbTvNzNVkL0=",
        version = "v0.0.0-20230328101604-8cdbc5f3d279",
    )
    go_repository(
        name = "com_github_tyler_smith_go_bip39",
        importpath = "github.com/tyler-smith/go-bip39",
        sum = "h1:5eUemwrMargf3BSLRRCalXT93Ns6pQJIjYQN2nyfOP8=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_ugorji_go_codec",
        importpath = "github.com/ugorji/go/codec",
        sum = "h1:YPXUKf7fYbp/y8xloBqZOw2qaVggbfwMlI8WM3wZUJ0=",
        version = "v1.2.7",
    )
    go_repository(
        name = "com_github_urfave_cli",
        importpath = "github.com/urfave/cli",
        sum = "h1:gsqYFH8bb9ekPA12kRo0hfjngWQjkJPlN9R0N78BoUo=",
        version = "v1.22.2",
    )
    go_repository(
        name = "com_github_urfave_cli_v2",
        importpath = "github.com/urfave/cli/v2",
        sum = "h1:3f3AMg3HpThFNT4I++TKOejZO8yU55t3JnnSr4S4QEI=",
        version = "v2.26.0",
    )
    go_repository(
        name = "com_github_urfave_negroni",
        importpath = "github.com/urfave/negroni",
        sum = "h1:kIimOitoypq34K7TG7DUaJ9kq/N4Ofuwi1sjz0KipXc=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_uudashr_gocognit",
        importpath = "github.com/uudashr/gocognit",
        sum = "h1:rrSex7oHr3/pPLQ0xoWq108XMU8s678FJcQ+aSfOHa4=",
        version = "v1.0.5",
    )
    go_repository(
        name = "com_github_valyala_bytebufferpool",
        importpath = "github.com/valyala/bytebufferpool",
        sum = "h1:GqA5TC/0021Y/b9FG4Oi9Mr3q7XYx6KllzawFIhcdPw=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_valyala_fasthttp",
        importpath = "github.com/valyala/fasthttp",
        sum = "h1:CRq/00MfruPGFLTQKY8b+8SfdK60TxNztjRMnH0t1Yc=",
        version = "v1.40.0",
    )
    go_repository(
        name = "com_github_valyala_fasttemplate",
        importpath = "github.com/valyala/fasttemplate",
        sum = "h1:lxLXG0uE3Qnshl9QyaK6XJxMXlQZELvChBOCmQD0Loo=",
        version = "v1.2.2",
    )
    go_repository(
        name = "com_github_viant_assertly",
        importpath = "github.com/viant/assertly",
        sum = "h1:5x1GzBaRteIwTr5RAGFVG14uNeRFxVNbXPWrK2qAgpc=",
        version = "v0.4.8",
    )
    go_repository(
        name = "com_github_viant_toolbox",
        importpath = "github.com/viant/toolbox",
        sum = "h1:6TteTDQ68CjgcCe8wH3D3ZhUQQOJXMTbj/D9rkk2a1k=",
        version = "v0.24.0",
    )
    go_repository(
        name = "com_github_victoriametrics_fastcache",
        importpath = "github.com/VictoriaMetrics/fastcache",
        sum = "h1:N0y9ASrJ0F6h0QaC3o6uJb3NIZ9VKLjCM7NQbSmF7WI=",
        version = "v1.12.2",
    )
    go_repository(
        name = "com_github_vividcortex_gohistogram",
        importpath = "github.com/VividCortex/gohistogram",
        sum = "h1:6+hBz+qvs0JOrrNhhmR7lFxo5sINxBCGXrdtl/UvroE=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_vmihailenco_msgpack_v5",
        importpath = "github.com/vmihailenco/msgpack/v5",
        sum = "h1:5gO0H1iULLWGhs2H5tbAHIZTV8/cYafcFOr9znI5mJU=",
        version = "v5.3.5",
    )
    go_repository(
        name = "com_github_vmihailenco_tagparser_v2",
        importpath = "github.com/vmihailenco/tagparser/v2",
        sum = "h1:y09buUbR+b5aycVFQs/g70pqKVZNBmxwAhO7/IwNM9g=",
        version = "v2.0.0",
    )
    go_repository(
        name = "com_github_wealdtech_go_bytesutil",
        importpath = "github.com/wealdtech/go-bytesutil",
        sum = "h1:ocEg3Ke2GkZ4vQw5lp46rmO+pfqCCTgq35gqOy8JKVc=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_wealdtech_go_eth2_types_v2",
        build_directives = [
            "gazelle:resolve go github.com/herumi/bls-eth-go-binary/bls @herumi_bls_eth_go_binary//:go_default_library",
        ],
        importpath = "github.com/wealdtech/go-eth2-types/v2",
        sum = "h1:tiA6T88M6XQIbrV5Zz53l1G5HtRERcxQfmET225V4Ls=",
        version = "v2.5.2",
    )
    go_repository(
        name = "com_github_wealdtech_go_eth2_util",
        importpath = "github.com/wealdtech/go-eth2-util",
        sum = "h1:2INPeOR35x5LdFFpSzyw954WzTD+DFyHe3yKlJnG5As=",
        version = "v1.6.3",
    )
    go_repository(
        name = "com_github_wealdtech_go_eth2_wallet_encryptor_keystorev4",
        importpath = "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4",
        sum = "h1:SxrDVSr+oXuT1x8kZt4uWqNCvv5xXEGV9zd7cuSrZS8=",
        version = "v1.1.3",
    )
    go_repository(
        name = "com_github_wealdtech_go_eth2_wallet_types_v2",
        importpath = "github.com/wealdtech/go-eth2-wallet-types/v2",
        sum = "h1:264/meVYWt1wFw6Mtn+xwkZkXjID42gNra4rycoiDXI=",
        version = "v2.8.2",
    )
    go_repository(
        name = "com_github_wlynxg_anet",
        importpath = "github.com/wlynxg/anet",
        sum = "h1:0de1OFQxnNqAu+x2FAKKCVIrnfGKQbs7FQz++tB0+Uw=",
        version = "v0.0.4",
    )
    go_repository(
        name = "com_github_x448_float16",
        importpath = "github.com/x448/float16",
        sum = "h1:qLwI1I70+NjRFUR3zs1JPUCgaCXSh3SW62uAKT1mSBM=",
        version = "v0.8.4",
    )
    go_repository(
        name = "com_github_xdg_scram",
        importpath = "github.com/xdg/scram",
        sum = "h1:u40Z8hqBAAQyv+vATcGgV0YCnDjqSL7/q/JyPhhJSPk=",
        version = "v0.0.0-20180814205039-7eeb5667e42c",
    )
    go_repository(
        name = "com_github_xdg_stringprep",
        importpath = "github.com/xdg/stringprep",
        sum = "h1:d9X0esnoa3dFsV0FG35rAT0RIhYFlPq7MiP+DW89La0=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_xhit_go_str2duration_v2",
        importpath = "github.com/xhit/go-str2duration/v2",
        sum = "h1:lxklc02Drh6ynqX+DdPyp5pCKLUQpRT8bp8Ydu2Bstc=",
        version = "v2.1.0",
    )
    go_repository(
        name = "com_github_xiang90_probing",
        importpath = "github.com/xiang90/probing",
        sum = "h1:eY9dn8+vbi4tKz5Qo6v2eYzo7kUS51QINcR5jNpbZS8=",
        version = "v0.0.0-20190116061207-43a291ad63a2",
    )
    go_repository(
        name = "com_github_xrash_smetrics",
        importpath = "github.com/xrash/smetrics",
        sum = "h1:bAn7/zixMGCfxrRTfdpNzjtPYqr8smhKouy9mxVdGPU=",
        version = "v0.0.0-20201216005158-039620a65673",
    )
    go_repository(
        name = "com_github_xtaci_kcp_go",
        importpath = "github.com/xtaci/kcp-go",
        sum = "h1:TN1uey3Raw0sTz0Fg8GkfM0uH3YwzhnZWQ1bABv5xAg=",
        version = "v5.4.20+incompatible",
    )
    go_repository(
        name = "com_github_xtaci_lossyconn",
        importpath = "github.com/xtaci/lossyconn",
        sum = "h1:J0GxkO96kL4WF+AIT3M4mfUVinOCPgf2uUWYFUzN0sM=",
        version = "v0.0.0-20190602105132-8df528c0c9ae",
    )
    go_repository(
        name = "com_github_yosssi_ace",
        importpath = "github.com/yosssi/ace",
        sum = "h1:tUkIP/BLdKqrlrPwcmH0shwEEhTRHoGnc1wFIWmaBUA=",
        version = "v0.0.5",
    )
    go_repository(
        name = "com_github_yuin_goldmark",
        importpath = "github.com/yuin/goldmark",
        sum = "h1:fVcFKWvrslecOb/tg+Cc05dkeYx540o0FuFt3nUVDoE=",
        version = "v1.4.13",
    )
    go_repository(
        name = "com_github_yusufpapurcu_wmi",
        importpath = "github.com/yusufpapurcu/wmi",
        sum = "h1:E1ctvB7uKFMOJw3fdOW32DwGE9I7t++CRUEMKvFoFiw=",
        version = "v1.2.3",
    )
    go_repository(
        name = "com_google_cloud_go",
        importpath = "cloud.google.com/go",
        sum = "h1:Zc8gqp3+a9/Eyph2KDmcGaPtbKRIoqq4YTlL4NMD0Ys=",
        version = "v0.110.0",
    )
    go_repository(
        name = "com_google_cloud_go_accessapproval",
        importpath = "cloud.google.com/go/accessapproval",
        sum = "h1:x0cEHro/JFPd7eS4BlEWNTMecIj2HdXjOVB5BtvwER0=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_accesscontextmanager",
        importpath = "cloud.google.com/go/accesscontextmanager",
        sum = "h1:MG60JgnEoawHJrbWw0jGdv6HLNSf6gQvYRiXpuzqgEA=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_aiplatform",
        importpath = "cloud.google.com/go/aiplatform",
        sum = "h1:zTw+suCVchgZyO+k847wjzdVjWmrAuehxdvcZvJwfGg=",
        version = "v1.37.0",
    )
    go_repository(
        name = "com_google_cloud_go_analytics",
        importpath = "cloud.google.com/go/analytics",
        sum = "h1:LqAo3tAh2FU9+w/r7vc3hBjU23Kv7GhO/PDIW7kIYgM=",
        version = "v0.19.0",
    )
    go_repository(
        name = "com_google_cloud_go_apigateway",
        importpath = "cloud.google.com/go/apigateway",
        sum = "h1:ZI9mVO7x3E9RK/BURm2p1aw9YTBSCQe3klmyP1WxWEg=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_apigeeconnect",
        importpath = "cloud.google.com/go/apigeeconnect",
        sum = "h1:sWOmgDyAsi1AZ48XRHcATC0tsi9SkPT7DA/+VCfkaeA=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_apigeeregistry",
        importpath = "cloud.google.com/go/apigeeregistry",
        sum = "h1:E43RdhhCxdlV+I161gUY2rI4eOaMzHTA5kNkvRsFXvc=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_apikeys",
        importpath = "cloud.google.com/go/apikeys",
        sum = "h1:B9CdHFZTFjVti89tmyXXrO+7vSNo2jvZuHG8zD5trdQ=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_appengine",
        importpath = "cloud.google.com/go/appengine",
        sum = "h1:aBGDKmRIaRRoWJ2tAoN0oVSHoWLhtO9aj/NvUyP4aYs=",
        version = "v1.7.1",
    )
    go_repository(
        name = "com_google_cloud_go_area120",
        importpath = "cloud.google.com/go/area120",
        sum = "h1:ugckkFh4XkHJMPhTIx0CyvdoBxmOpMe8rNs4Ok8GAag=",
        version = "v0.7.1",
    )
    go_repository(
        name = "com_google_cloud_go_artifactregistry",
        importpath = "cloud.google.com/go/artifactregistry",
        sum = "h1:o1Q80vqEB6Qp8WLEH3b8FBLNUCrGQ4k5RFj0sn/sgO8=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_asset",
        importpath = "cloud.google.com/go/asset",
        sum = "h1:YAsssO08BqZ6mncbb6FPlj9h6ACS7bJQUOlzciSfbNk=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_assuredworkloads",
        importpath = "cloud.google.com/go/assuredworkloads",
        sum = "h1:VLGnVFta+N4WM+ASHbhc14ZOItOabDLH1MSoDv+Xuag=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_automl",
        importpath = "cloud.google.com/go/automl",
        sum = "h1:50VugllC+U4IGl3tDNcZaWvApHBTrn/TvyHDJ0wM+Uw=",
        version = "v1.12.0",
    )
    go_repository(
        name = "com_google_cloud_go_baremetalsolution",
        importpath = "cloud.google.com/go/baremetalsolution",
        sum = "h1:2AipdYXL0VxMboelTTw8c1UJ7gYu35LZYUbuRv9Q28s=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_batch",
        importpath = "cloud.google.com/go/batch",
        sum = "h1:YbMt0E6BtqeD5FvSv1d56jbVsWEzlGm55lYte+M6Mzs=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_beyondcorp",
        importpath = "cloud.google.com/go/beyondcorp",
        sum = "h1:UkY2BTZkEUAVrgqnSdOJ4p3y9ZRBPEe1LkjgC8Bj/Pc=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_bigquery",
        importpath = "cloud.google.com/go/bigquery",
        sum = "h1:RscMV6LbnAmhAzD893Lv9nXXy2WCaJmbxYPWDLbGqNQ=",
        version = "v1.50.0",
    )
    go_repository(
        name = "com_google_cloud_go_billing",
        importpath = "cloud.google.com/go/billing",
        sum = "h1:JYj28UYF5w6VBAh0gQYlgHJ/OD1oA+JgW29YZQU+UHM=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_binaryauthorization",
        importpath = "cloud.google.com/go/binaryauthorization",
        sum = "h1:d3pMDBCCNivxt5a4eaV7FwL7cSH0H7RrEnFrTb1QKWs=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_certificatemanager",
        importpath = "cloud.google.com/go/certificatemanager",
        sum = "h1:5C5UWeSt8Jkgp7OWn2rCkLmYurar/vIWIoSQ2+LaTOc=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_channel",
        importpath = "cloud.google.com/go/channel",
        sum = "h1:GpcQY5UJKeOekYgsX3QXbzzAc/kRGtBq43fTmyKe6Uw=",
        version = "v1.12.0",
    )
    go_repository(
        name = "com_google_cloud_go_cloudbuild",
        importpath = "cloud.google.com/go/cloudbuild",
        sum = "h1:GHQCjV4WlPPVU/j3Rlpc8vNIDwThhd1U9qSY/NPZdko=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_clouddms",
        importpath = "cloud.google.com/go/clouddms",
        sum = "h1:E7v4TpDGUyEm1C/4KIrpVSOCTm0P6vWdHT0I4mostRA=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_cloudtasks",
        importpath = "cloud.google.com/go/cloudtasks",
        sum = "h1:uK5k6abf4yligFgYFnG0ni8msai/dSv6mDmiBulU0hU=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_compute",
        importpath = "cloud.google.com/go/compute",
        sum = "h1:+9zda3WGgW1ZSTlVppLCYFIr48Pa35q1uG2N1itbCEQ=",
        version = "v1.19.0",
    )
    go_repository(
        name = "com_google_cloud_go_compute_metadata",
        importpath = "cloud.google.com/go/compute/metadata",
        sum = "h1:Tz+eQXMEqDIKRsmY3cHTL6FVaynIjX2QxYC4trgAKZc=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_google_cloud_go_contactcenterinsights",
        importpath = "cloud.google.com/go/contactcenterinsights",
        sum = "h1:jXIpfcH/VYSE1SYcPzO0n1VVb+sAamiLOgCw45JbOQk=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_container",
        importpath = "cloud.google.com/go/container",
        sum = "h1:NKlY/wCDapfVZlbVVaeuu2UZZED5Dy1z4Zx1KhEzm8c=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_containeranalysis",
        importpath = "cloud.google.com/go/containeranalysis",
        sum = "h1:EQ4FFxNaEAg8PqQCO7bVQfWz9NVwZCUKaM1b3ycfx3U=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_datacatalog",
        importpath = "cloud.google.com/go/datacatalog",
        sum = "h1:4H5IJiyUE0X6ShQBqgFFZvGGcrwGVndTwUSLP4c52gw=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataflow",
        importpath = "cloud.google.com/go/dataflow",
        sum = "h1:eYyD9o/8Nm6EttsKZaEGD84xC17bNgSKCu0ZxwqUbpg=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataform",
        importpath = "cloud.google.com/go/dataform",
        sum = "h1:Dyk+fufup1FR6cbHjFpMuP4SfPiF3LI3JtoIIALoq48=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_datafusion",
        importpath = "cloud.google.com/go/datafusion",
        sum = "h1:sZjRnS3TWkGsu1LjYPFD/fHeMLZNXDK6PDHi2s2s/bk=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_datalabeling",
        importpath = "cloud.google.com/go/datalabeling",
        sum = "h1:ch4qA2yvddGRUrlfwrNJCr79qLqhS9QBwofPHfFlDIk=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataplex",
        importpath = "cloud.google.com/go/dataplex",
        sum = "h1:RvoZ5T7gySwm1CHzAw7yY1QwwqaGswunmqEssPxU/AM=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataproc",
        importpath = "cloud.google.com/go/dataproc",
        sum = "h1:W47qHL3W4BPkAIbk4SWmIERwsWBaNnWm0P2sdx3YgGU=",
        version = "v1.12.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataqna",
        importpath = "cloud.google.com/go/dataqna",
        sum = "h1:yFzi/YU4YAdjyo7pXkBE2FeHbgz5OQQBVDdbErEHmVQ=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_datastore",
        importpath = "cloud.google.com/go/datastore",
        sum = "h1:iF6I/HaLs3Ado8uRKMvZRvF/ZLkWaWE9i8AiHzbC774=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_datastream",
        importpath = "cloud.google.com/go/datastream",
        sum = "h1:BBCBTnWMDwwEzQQmipUXxATa7Cm7CA/gKjKcR2w35T0=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_deploy",
        importpath = "cloud.google.com/go/deploy",
        sum = "h1:otshdKEbmsi1ELYeCKNYppwV0UH5xD05drSdBm7ouTk=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_dialogflow",
        importpath = "cloud.google.com/go/dialogflow",
        sum = "h1:uVlKKzp6G/VtSW0E7IH1Y5o0H48/UOCmqksG2riYCwQ=",
        version = "v1.32.0",
    )
    go_repository(
        name = "com_google_cloud_go_dlp",
        importpath = "cloud.google.com/go/dlp",
        sum = "h1:1JoJqezlgu6NWCroBxr4rOZnwNFILXr4cB9dMaSKO4A=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_documentai",
        importpath = "cloud.google.com/go/documentai",
        sum = "h1:KM3Xh0QQyyEdC8Gs2vhZfU+rt6OCPF0dwVwxKgLmWfI=",
        version = "v1.18.0",
    )
    go_repository(
        name = "com_google_cloud_go_domains",
        importpath = "cloud.google.com/go/domains",
        sum = "h1:2ti/o9tlWL4N+wIuWUNH+LbfgpwxPr8J1sv9RHA4bYQ=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_edgecontainer",
        importpath = "cloud.google.com/go/edgecontainer",
        sum = "h1:O0YVE5v+O0Q/ODXYsQHmHb+sYM8KNjGZw2pjX2Ws41c=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_google_cloud_go_errorreporting",
        importpath = "cloud.google.com/go/errorreporting",
        sum = "h1:kj1XEWMu8P0qlLhm3FwcaFsUvXChV/OraZwA70trRR0=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_google_cloud_go_essentialcontacts",
        importpath = "cloud.google.com/go/essentialcontacts",
        sum = "h1:gIzEhCoOT7bi+6QZqZIzX1Erj4SswMPIteNvYVlu+pM=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_eventarc",
        importpath = "cloud.google.com/go/eventarc",
        sum = "h1:fsJmNeqvqtk74FsaVDU6cH79lyZNCYP8Rrv7EhaB/PU=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_filestore",
        importpath = "cloud.google.com/go/filestore",
        sum = "h1:ckTEXN5towyTMu4q0uQ1Mde/JwTHur0gXs8oaIZnKfw=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_firestore",
        importpath = "cloud.google.com/go/firestore",
        sum = "h1:IBlRyxgGySXu5VuW0RgGFlTtLukSnNkpDiEOMkQkmpA=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_functions",
        importpath = "cloud.google.com/go/functions",
        sum = "h1:pPDqtsXG2g9HeOQLoquLbmvmb82Y4Ezdo1GXuotFoWg=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_gaming",
        importpath = "cloud.google.com/go/gaming",
        sum = "h1:7vEhFnZmd931Mo7sZ6pJy7uQPDxF7m7v8xtBheG08tc=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_gkebackup",
        importpath = "cloud.google.com/go/gkebackup",
        sum = "h1:za3QZvw6ujR0uyqkhomKKKNoXDyqYGPJies3voUK8DA=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_google_cloud_go_gkeconnect",
        importpath = "cloud.google.com/go/gkeconnect",
        sum = "h1:gXYKciHS/Lgq0GJ5Kc9SzPA35NGc3yqu6SkjonpEr2Q=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_gkehub",
        importpath = "cloud.google.com/go/gkehub",
        sum = "h1:TqCSPsEBQ6oZSJgEYZ3XT8x2gUadbvfwI32YB0kuHCs=",
        version = "v0.12.0",
    )
    go_repository(
        name = "com_google_cloud_go_gkemulticloud",
        importpath = "cloud.google.com/go/gkemulticloud",
        sum = "h1:8I84Q4vl02rJRsFiinBxl7WCozfdLlUVBQuSrqr9Wtk=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_gsuiteaddons",
        importpath = "cloud.google.com/go/gsuiteaddons",
        sum = "h1:1mvhXqJzV0Vg5Fa95QwckljODJJfDFXV4pn+iL50zzA=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_iam",
        importpath = "cloud.google.com/go/iam",
        sum = "h1:+CmB+K0J/33d0zSQ9SlFWUeCCEn5XJA0ZMZ3pHE9u8k=",
        version = "v0.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_iap",
        importpath = "cloud.google.com/go/iap",
        sum = "h1:PxVHFuMxmSZyfntKXHXhd8bo82WJ+LcATenq7HLdVnU=",
        version = "v1.7.1",
    )
    go_repository(
        name = "com_google_cloud_go_ids",
        importpath = "cloud.google.com/go/ids",
        sum = "h1:fodnCDtOXuMmS8LTC2y3h8t24U8F3eKWfhi+3LY6Qf0=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_google_cloud_go_iot",
        importpath = "cloud.google.com/go/iot",
        sum = "h1:39W5BFSarRNZfVG0eXI5LYux+OVQT8GkgpHCnrZL2vM=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_kms",
        importpath = "cloud.google.com/go/kms",
        sum = "h1:7hm1bRqGCA1GBRQUrp831TwJ9TWhP+tvLuP497CQS2g=",
        version = "v1.10.1",
    )
    go_repository(
        name = "com_google_cloud_go_language",
        importpath = "cloud.google.com/go/language",
        sum = "h1:7Ulo2mDk9huBoBi8zCE3ONOoBrL6UXfAI71CLQ9GEIM=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_lifesciences",
        importpath = "cloud.google.com/go/lifesciences",
        sum = "h1:uWrMjWTsGjLZpCTWEAzYvyXj+7fhiZST45u9AgasasI=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_logging",
        importpath = "cloud.google.com/go/logging",
        sum = "h1:CJYxlNNNNAMkHp9em/YEXcfJg+rPDg7YfwoRpMU+t5I=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_longrunning",
        importpath = "cloud.google.com/go/longrunning",
        sum = "h1:v+yFJOfKC3yZdY6ZUI933pIYdhyhV8S3NpWrXWmg7jM=",
        version = "v0.4.1",
    )
    go_repository(
        name = "com_google_cloud_go_managedidentities",
        importpath = "cloud.google.com/go/managedidentities",
        sum = "h1:ZRQ4k21/jAhrHBVKl/AY7SjgzeJwG1iZa+mJ82P+VNg=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_maps",
        importpath = "cloud.google.com/go/maps",
        sum = "h1:mv9YaczD4oZBZkM5XJl6fXQ984IkJNHPwkc8MUsdkBo=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_mediatranslation",
        importpath = "cloud.google.com/go/mediatranslation",
        sum = "h1:anPxH+/WWt8Yc3EdoEJhPMBRF7EhIdz426A+tuoA0OU=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_memcache",
        importpath = "cloud.google.com/go/memcache",
        sum = "h1:8/VEmWCpnETCrBwS3z4MhT+tIdKgR1Z4Tr2tvYH32rg=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_metastore",
        importpath = "cloud.google.com/go/metastore",
        sum = "h1:QCFhZVe2289KDBQ7WxaHV2rAmPrmRAdLC6gbjUd3HPo=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_monitoring",
        importpath = "cloud.google.com/go/monitoring",
        sum = "h1:2qsrgXGVoRXpP7otZ14eE1I568zAa92sJSDPyOJvwjM=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_networkconnectivity",
        importpath = "cloud.google.com/go/networkconnectivity",
        sum = "h1:ZD6b4Pk1jEtp/cx9nx0ZYcL3BKqDa+KixNDZ6Bjs1B8=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_networkmanagement",
        importpath = "cloud.google.com/go/networkmanagement",
        sum = "h1:8KWEUNGcpSX9WwZXq7FtciuNGPdPdPN/ruDm769yAEM=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_networksecurity",
        importpath = "cloud.google.com/go/networksecurity",
        sum = "h1:sOc42Ig1K2LiKlzG71GUVloeSJ0J3mffEBYmvu+P0eo=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_notebooks",
        importpath = "cloud.google.com/go/notebooks",
        sum = "h1:Kg2K3K7CbSXYJHZ1aGQpf1xi5x2GUvQWf2sFVuiZh8M=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_optimization",
        importpath = "cloud.google.com/go/optimization",
        sum = "h1:dj8O4VOJRB4CUwZXdmwNViH1OtI0WtWL867/lnYH248=",
        version = "v1.3.1",
    )
    go_repository(
        name = "com_google_cloud_go_orchestration",
        importpath = "cloud.google.com/go/orchestration",
        sum = "h1:Vw+CEXo8M/FZ1rb4EjcLv0gJqqw89b7+g+C/EmniTb8=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_orgpolicy",
        importpath = "cloud.google.com/go/orgpolicy",
        sum = "h1:XDriMWug7sd0kYT1QKofRpRHzjad0bK8Q8uA9q+XrU4=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_osconfig",
        importpath = "cloud.google.com/go/osconfig",
        sum = "h1:PkSQx4OHit5xz2bNyr11KGcaFccL5oqglFPdTboyqwQ=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_oslogin",
        importpath = "cloud.google.com/go/oslogin",
        sum = "h1:whP7vhpmc+ufZa90eVpkfbgzJRK/Xomjz+XCD4aGwWw=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_phishingprotection",
        importpath = "cloud.google.com/go/phishingprotection",
        sum = "h1:l6tDkT7qAEV49MNEJkEJTB6vOO/onbSOcNtAT09HPuA=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_policytroubleshooter",
        importpath = "cloud.google.com/go/policytroubleshooter",
        sum = "h1:yKAGC4p9O61ttZUswaq9GAn1SZnEzTd0vUYXD7ZBT7Y=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_privatecatalog",
        importpath = "cloud.google.com/go/privatecatalog",
        sum = "h1:EPEJ1DpEGXLDnmc7mnCAqFmkwUJbIsaLAiLHVOkkwtc=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_pubsub",
        importpath = "cloud.google.com/go/pubsub",
        sum = "h1:vCge8m7aUKBJYOgrZp7EsNDf6QMd2CAlXZqWTn3yq6s=",
        version = "v1.30.0",
    )
    go_repository(
        name = "com_google_cloud_go_pubsublite",
        importpath = "cloud.google.com/go/pubsublite",
        sum = "h1:cb9fsrtpINtETHiJ3ECeaVzrfIVhcGjhhJEjybHXHao=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_recaptchaenterprise_v2",
        importpath = "cloud.google.com/go/recaptchaenterprise/v2",
        sum = "h1:6iOCujSNJ0YS7oNymI64hXsjGq60T4FK1zdLugxbzvU=",
        version = "v2.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_recommendationengine",
        importpath = "cloud.google.com/go/recommendationengine",
        sum = "h1:VibRFCwWXrFebEWKHfZAt2kta6pS7Tlimsnms0fjv7k=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_recommender",
        importpath = "cloud.google.com/go/recommender",
        sum = "h1:ZnFRY5R6zOVk2IDS1Jbv5Bw+DExCI5rFumsTnMXiu/A=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_redis",
        importpath = "cloud.google.com/go/redis",
        sum = "h1:JoAd3SkeDt3rLFAAxEvw6wV4t+8y4ZzfZcZmddqphQ8=",
        version = "v1.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_resourcemanager",
        importpath = "cloud.google.com/go/resourcemanager",
        sum = "h1:NRM0p+RJkaQF9Ee9JMnUV9BQ2QBIOq/v8M+Pbv/wmCs=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_resourcesettings",
        importpath = "cloud.google.com/go/resourcesettings",
        sum = "h1:8Dua37kQt27CCWHm4h/Q1XqCF6ByD7Ouu49xg95qJzI=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_retail",
        importpath = "cloud.google.com/go/retail",
        sum = "h1:1Dda2OpFNzIb4qWgFZjYlpP7sxX3aLeypKG6A3H4Yys=",
        version = "v1.12.0",
    )
    go_repository(
        name = "com_google_cloud_go_run",
        importpath = "cloud.google.com/go/run",
        sum = "h1:ydJQo+k+MShYnBfhaRHSZYeD/SQKZzZLAROyfpeD9zw=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_scheduler",
        importpath = "cloud.google.com/go/scheduler",
        sum = "h1:NpQAHtx3sulByTLe2dMwWmah8PWgeoieFPpJpArwFV0=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_secretmanager",
        importpath = "cloud.google.com/go/secretmanager",
        sum = "h1:pu03bha7ukxF8otyPKTFdDz+rr9sE3YauS5PliDXK60=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_security",
        importpath = "cloud.google.com/go/security",
        sum = "h1:PYvDxopRQBfYAXKAuDpFCKBvDOWPWzp9k/H5nB3ud3o=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_securitycenter",
        importpath = "cloud.google.com/go/securitycenter",
        sum = "h1:AF3c2s3awNTMoBtMX3oCUoOMmGlYxGOeuXSYHNBkf14=",
        version = "v1.19.0",
    )
    go_repository(
        name = "com_google_cloud_go_servicecontrol",
        importpath = "cloud.google.com/go/servicecontrol",
        sum = "h1:d0uV7Qegtfaa7Z2ClDzr9HJmnbJW7jn0WhZ7wOX6hLE=",
        version = "v1.11.1",
    )
    go_repository(
        name = "com_google_cloud_go_servicedirectory",
        importpath = "cloud.google.com/go/servicedirectory",
        sum = "h1:SJwk0XX2e26o25ObYUORXx6torSFiYgsGkWSkZgkoSU=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_servicemanagement",
        importpath = "cloud.google.com/go/servicemanagement",
        sum = "h1:fopAQI/IAzlxnVeiKn/8WiV6zKndjFkvi+gzu+NjywY=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_serviceusage",
        importpath = "cloud.google.com/go/serviceusage",
        sum = "h1:rXyq+0+RSIm3HFypctp7WoXxIA563rn206CfMWdqXX4=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_shell",
        importpath = "cloud.google.com/go/shell",
        sum = "h1:wT0Uw7ib7+AgZST9eCDygwTJn4+bHMDtZo5fh7kGWDU=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_spanner",
        importpath = "cloud.google.com/go/spanner",
        sum = "h1:7VdjZ8zj4sHbDw55atp5dfY6kn1j9sam9DRNpPQhqR4=",
        version = "v1.45.0",
    )
    go_repository(
        name = "com_google_cloud_go_speech",
        importpath = "cloud.google.com/go/speech",
        sum = "h1:JEVoWGNnTF128kNty7T4aG4eqv2z86yiMJPT9Zjp+iw=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_storage",
        importpath = "cloud.google.com/go/storage",
        sum = "h1:6RRlFMv1omScs6iq2hfE3IvgE+l6RfJPampq8UZc5TU=",
        version = "v1.14.0",
    )
    go_repository(
        name = "com_google_cloud_go_storagetransfer",
        importpath = "cloud.google.com/go/storagetransfer",
        sum = "h1:5T+PM+3ECU3EY2y9Brv0Sf3oka8pKmsCfpQ07+91G9o=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_talent",
        importpath = "cloud.google.com/go/talent",
        sum = "h1:nI9sVZPjMKiO2q3Uu0KhTDVov3Xrlpt63fghP9XjyEM=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_texttospeech",
        importpath = "cloud.google.com/go/texttospeech",
        sum = "h1:H4g1ULStsbVtalbZGktyzXzw6jP26RjVGYx9RaYjBzc=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_tpu",
        importpath = "cloud.google.com/go/tpu",
        sum = "h1:/34T6CbSi+kTv5E19Q9zbU/ix8IviInZpzwz3rsFE+A=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_trace",
        importpath = "cloud.google.com/go/trace",
        sum = "h1:olxC0QHC59zgJVALtgqfD9tGk0lfeCP5/AGXL3Px/no=",
        version = "v1.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_translate",
        importpath = "cloud.google.com/go/translate",
        sum = "h1:GvLP4oQ4uPdChBmBaUSa/SaZxCdyWELtlAaKzpHsXdA=",
        version = "v1.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_video",
        importpath = "cloud.google.com/go/video",
        sum = "h1:upIbnGI0ZgACm58HPjAeBMleW3sl5cT84AbYQ8PWOgM=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_videointelligence",
        importpath = "cloud.google.com/go/videointelligence",
        sum = "h1:Uh5BdoET8XXqXX2uXIahGb+wTKbLkGH7s4GXR58RrG8=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_vision_v2",
        importpath = "cloud.google.com/go/vision/v2",
        sum = "h1:8C8RXUJoflCI4yVdqhTy9tRyygSHmp60aP363z23HKg=",
        version = "v2.7.0",
    )
    go_repository(
        name = "com_google_cloud_go_vmmigration",
        importpath = "cloud.google.com/go/vmmigration",
        sum = "h1:Azs5WKtfOC8pxvkyrDvt7J0/4DYBch0cVbuFfCCFt5k=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_vmwareengine",
        importpath = "cloud.google.com/go/vmwareengine",
        sum = "h1:b0NBu7S294l0gmtrT0nOJneMYgZapr5x9tVWvgDoVEM=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_google_cloud_go_vpcaccess",
        importpath = "cloud.google.com/go/vpcaccess",
        sum = "h1:FOe6CuiQD3BhHJWt7E8QlbBcaIzVRddupwJlp7eqmn4=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_webrisk",
        importpath = "cloud.google.com/go/webrisk",
        sum = "h1:IY+L2+UwxcVm2zayMAtBhZleecdIFLiC+QJMzgb0kT0=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_websecurityscanner",
        importpath = "cloud.google.com/go/websecurityscanner",
        sum = "h1:AHC1xmaNMOZtNqxI9Rmm87IJEyPaRkOxeI0gpAacXGk=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_google_cloud_go_workflows",
        importpath = "cloud.google.com/go/workflows",
        sum = "h1:FfGp9w0cYnaKZJhUOMqCOJCYT/WlvYBfTQhFWV3sRKI=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_lukechampine_blake3",
        importpath = "lukechampine.com/blake3",
        sum = "h1:sJ3XhFINmHSrYCgl958hscfIa3bw8x4DqMP3u1YvoYE=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_shuralyov_dmitri_app_changes",
        importpath = "dmitri.shuralyov.com/app/changes",
        sum = "h1:hJiie5Bf3QucGRa4ymsAUOxyhYwGEz1xrsVk0P8erlw=",
        version = "v0.0.0-20180602232624-0a106ad413e3",
    )
    go_repository(
        name = "com_shuralyov_dmitri_gpu_mtl",
        importpath = "dmitri.shuralyov.com/gpu/mtl",
        sum = "h1:VpgP7xuJadIUuKccphEpTJnWhS2jkQyMt6Y7pJCD7fY=",
        version = "v0.0.0-20190408044501-666a987793e9",
    )
    go_repository(
        name = "com_shuralyov_dmitri_html_belt",
        importpath = "dmitri.shuralyov.com/html/belt",
        sum = "h1:SPOUaucgtVls75mg+X7CXigS71EnsfVUK/2CgVrwqgw=",
        version = "v0.0.0-20180602232347-f7d459c86be0",
    )
    go_repository(
        name = "com_shuralyov_dmitri_service_change",
        importpath = "dmitri.shuralyov.com/service/change",
        sum = "h1:GvWw74lx5noHocd+f6HBMXK6DuggBB1dhVkuGZbv7qM=",
        version = "v0.0.0-20181023043359-a85b471d5412",
    )
    go_repository(
        name = "com_shuralyov_dmitri_state",
        importpath = "dmitri.shuralyov.com/state",
        sum = "h1:ivON6cwHK1OH26MZyWDCnbTRZZf0IhNsENoNAKFS1g4=",
        version = "v0.0.0-20180228185332-28bcc343414c",
    )
    go_repository(
        name = "com_sourcegraph_sourcegraph_appdash",
        importpath = "sourcegraph.com/sourcegraph/appdash",
        sum = "h1:ucqkfpjg9WzSUubAO62csmucvxl4/JeW3F4I4909XkM=",
        version = "v0.0.0-20190731080439-ebfcffb1b5c0",
    )
    go_repository(
        name = "com_sourcegraph_sourcegraph_go_diff",
        importpath = "sourcegraph.com/sourcegraph/go-diff",
        sum = "h1:eTiIR0CoWjGzJcnQ3OkhIl/b9GJovq4lSAVRt0ZFEG8=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_sourcegraph_sqs_pbtypes",
        importpath = "sourcegraph.com/sqs/pbtypes",
        sum = "h1:JPJh2pk3+X4lXAkZIk2RuE/7/FoK9maXw+TNPJhVS/c=",
        version = "v0.0.0-20180604144634-d3ebe8f20ae4",
    )
    go_repository(
        name = "dev_cel_expr",
        importpath = "cel.dev/expr",
        sum = "h1:O1jzfJCQBfL5BFoYktaxwIhuttaQPsVWerH9/EEKx0w=",
        version = "v0.15.0",
    )
    go_repository(
        name = "in_gopkg_alecthomas_kingpin_v2",
        importpath = "gopkg.in/alecthomas/kingpin.v2",
        sum = "h1:jMFz6MfLP0/4fUyZle81rXUoxOBFi19VUFKVDOQfozc=",
        version = "v2.2.6",
    )
    go_repository(
        name = "in_gopkg_bsm_ratelimit_v1",
        importpath = "gopkg.in/bsm/ratelimit.v1",
        sum = "h1:stTHdEoWg1pQ8riaP5ROrjS6zy6wewH/Q2iwnLCQUXY=",
        version = "v1.0.0-20160220154919-db14e161995a",
    )
    go_repository(
        name = "in_gopkg_cenkalti_backoff_v1",
        importpath = "gopkg.in/cenkalti/backoff.v1",
        sum = "h1:Arh75ttbsvlpVA7WtVpH4u9h6Zl46xuptxqLxPiSo4Y=",
        version = "v1.1.0",
    )
    go_repository(
        name = "in_gopkg_check_v1",
        importpath = "gopkg.in/check.v1",
        sum = "h1:Hei/4ADfdWqJk1ZMxUNpqntNwaWcugrBjAiHlqqRiVk=",
        version = "v1.0.0-20201130134442-10cb98267c6c",
    )
    go_repository(
        name = "in_gopkg_cheggaaa_pb_v1",
        importpath = "gopkg.in/cheggaaa/pb.v1",
        sum = "h1:Ev7yu1/f6+d+b3pi5vPdRPc6nNtP1umSfcWiEfRqv6I=",
        version = "v1.0.25",
    )
    go_repository(
        name = "in_gopkg_d4l3k_messagediff_v1",
        importpath = "gopkg.in/d4l3k/messagediff.v1",
        sum = "h1:70AthpjunwzUiarMHyED52mj9UwtAnE89l1Gmrt3EU0=",
        version = "v1.2.1",
    )
    go_repository(
        name = "in_gopkg_errgo_v2",
        importpath = "gopkg.in/errgo.v2",
        sum = "h1:0vLT13EuvQ0hNvakwLuFZ/jYrLp5F3kcWHXdRggjCE8=",
        version = "v2.1.0",
    )
    go_repository(
        name = "in_gopkg_fsnotify_v1",
        importpath = "gopkg.in/fsnotify.v1",
        sum = "h1:xOHLXZwVvI9hhs+cLKq5+I5onOuwQLhQwiu63xxlHs4=",
        version = "v1.4.7",
    )
    go_repository(
        name = "in_gopkg_gcfg_v1",
        importpath = "gopkg.in/gcfg.v1",
        sum = "h1:m8OOJ4ccYHnx2f4gQwpno8nAX5OGOh7RLaaz0pj3Ogs=",
        version = "v1.2.3",
    )
    go_repository(
        name = "in_gopkg_inf_v0",
        importpath = "gopkg.in/inf.v0",
        sum = "h1:73M5CoZyi3ZLMOyDlQh031Cx6N9NDJ2Vvfl76EDAgDc=",
        version = "v0.9.1",
    )
    go_repository(
        name = "in_gopkg_ini_v1",
        importpath = "gopkg.in/ini.v1",
        sum = "h1:Dgnx+6+nfE+IfzjUEISNeydPJh9AXNNsWbGP9KzCsOA=",
        version = "v1.67.0",
    )
    go_repository(
        name = "in_gopkg_jcmturner_aescts_v1",
        importpath = "gopkg.in/jcmturner/aescts.v1",
        sum = "h1:cVVZBK2b1zY26haWB4vbBiZrfFQnfbTVrE3xZq6hrEw=",
        version = "v1.0.1",
    )
    go_repository(
        name = "in_gopkg_jcmturner_dnsutils_v1",
        importpath = "gopkg.in/jcmturner/dnsutils.v1",
        sum = "h1:cIuC1OLRGZrld+16ZJvvZxVJeKPsvd5eUIvxfoN5hSM=",
        version = "v1.0.1",
    )
    go_repository(
        name = "in_gopkg_jcmturner_goidentity_v3",
        importpath = "gopkg.in/jcmturner/goidentity.v3",
        sum = "h1:1duIyWiTaYvVx3YX2CYtpJbUFd7/UuPYCfgXtQ3VTbI=",
        version = "v3.0.0",
    )
    go_repository(
        name = "in_gopkg_jcmturner_gokrb5_v7",
        importpath = "gopkg.in/jcmturner/gokrb5.v7",
        sum = "h1:a9tsXlIDD9SKxotJMK3niV7rPZAJeX2aD/0yg3qlIrg=",
        version = "v7.5.0",
    )
    go_repository(
        name = "in_gopkg_jcmturner_rpc_v1",
        importpath = "gopkg.in/jcmturner/rpc.v1",
        sum = "h1:QHIUxTX1ISuAv9dD2wJ9HWQVuWDX/Zc0PfeC2tjc4rU=",
        version = "v1.1.0",
    )
    go_repository(
        name = "in_gopkg_natefinch_lumberjack_v2",
        importpath = "gopkg.in/natefinch/lumberjack.v2",
        sum = "h1:1Lc07Kr7qY4U2YPouBjpCLxpiyxIVoxqXgkXLknAOE8=",
        version = "v2.0.0",
    )
    go_repository(
        name = "in_gopkg_redis_v4",
        importpath = "gopkg.in/redis.v4",
        sum = "h1:y3XbwQAiHwgNLUng56mgWYK39vsPqo8sT84XTEcxjr0=",
        version = "v4.2.4",
    )
    go_repository(
        name = "in_gopkg_resty_v1",
        importpath = "gopkg.in/resty.v1",
        sum = "h1:CuXP0Pjfw9rOuY6EP+UvtNvt5DSqHpIxILZKT/quCZI=",
        version = "v1.12.0",
    )
    go_repository(
        name = "in_gopkg_tomb_v1",
        importpath = "gopkg.in/tomb.v1",
        sum = "h1:uRGJdciOHaEIrze2W8Q3AKkepLTh2hOroT7a+7czfdQ=",
        version = "v1.0.0-20141024135613-dd632973f1e7",
    )
    go_repository(
        name = "in_gopkg_warnings_v0",
        importpath = "gopkg.in/warnings.v0",
        sum = "h1:wFXVbFY8DY5/xOe1ECiWdKCzZlxgshcYVNkBHstARME=",
        version = "v0.1.2",
    )
    go_repository(
        name = "in_gopkg_yaml_v2",
        importpath = "gopkg.in/yaml.v2",
        sum = "h1:D8xgwECY7CYvx+Y2n4sBz93Jn9JRvxdiyyo8CTfuKaY=",
        version = "v2.4.0",
    )
    go_repository(
        name = "in_gopkg_yaml_v3",
        importpath = "gopkg.in/yaml.v3",
        sum = "h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=",
        version = "v3.0.1",
    )
    go_repository(
        name = "io_etcd_go_bbolt",
        importpath = "go.etcd.io/bbolt",
        sum = "h1:/ecaJf0sk1l4l6V4awd65v2C3ILy7MSj+s/x1ADCIMU=",
        version = "v1.3.6",
    )
    go_repository(
        name = "io_etcd_go_etcd",
        importpath = "go.etcd.io/etcd",
        sum = "h1:VcrIfasaLFkyjk6KNlXQSzO+B0fZcnECiDrKJsfxka0=",
        version = "v0.0.0-20191023171146-3cf2f69b5738",
    )
    go_repository(
        name = "io_k8s_api",
        build_file_proto_mode = "disable_global",
        importpath = "k8s.io/api",
        sum = "h1:XASIELmW8w8q0i1Y4124LqPoWMycLjyQti/fdYHYjCs=",
        version = "v0.30.4",
    )
    go_repository(
        name = "io_k8s_apimachinery",
        build_file_proto_mode = "disable_global",
        importpath = "k8s.io/apimachinery",
        sum = "h1:5QHQI2tInzr8LsT4kU/2+fSeibH1eIHswNx480cqIoY=",
        version = "v0.30.4",
    )
    go_repository(
        name = "io_k8s_client_go",
        build_extra_args = ["-exclude=vendor"],
        build_naming_convention = "go_default_library",
        importpath = "k8s.io/client-go",
        sum = "h1:eculUe+HPQoPbixfwmaSZGsKcOf7D288tH6hDAdd+wY=",
        version = "v0.30.4",
    )
    go_repository(
        name = "io_k8s_klog_v2",
        importpath = "k8s.io/klog/v2",
        sum = "h1:QXU6cPEOIslTGvZaXvFWiP9VKyeet3sawzTOvdXb4Vw=",
        version = "v2.120.1",
    )
    go_repository(
        name = "io_k8s_kube_openapi",
        importpath = "k8s.io/kube-openapi",
        sum = "h1:BZqlfIlq5YbRMFko6/PM7FjZpUb45WallggurYhKGag=",
        version = "v0.0.0-20240228011516-70dd3763d340",
    )
    go_repository(
        name = "io_k8s_sigs_json",
        importpath = "sigs.k8s.io/json",
        sum = "h1:EDPBXCAspyGV4jQlpZSudPeMmr1bNJefnuqLsRAsHZo=",
        version = "v0.0.0-20221116044647-bc3834ca7abd",
    )
    go_repository(
        name = "io_k8s_sigs_structured_merge_diff_v4",
        importpath = "sigs.k8s.io/structured-merge-diff/v4",
        sum = "h1:150L+0vs/8DA78h1u02ooW1/fFq/Lwr+sGiqlzvrtq4=",
        version = "v4.4.1",
    )
    go_repository(
        name = "io_k8s_sigs_yaml",
        importpath = "sigs.k8s.io/yaml",
        sum = "h1:a2VclLzOGrwOHDiV8EfBGhvjHvP46CtW5j6POvhYGGo=",
        version = "v1.3.0",
    )
    go_repository(
        name = "io_k8s_utils",
        importpath = "k8s.io/utils",
        sum = "h1:sgn3ZU783SCgtaSJjpcVVlRqd6GSnlTLKgpAAttJvpI=",
        version = "v0.0.0-20230726121419-3b25d923346b",
    )
    go_repository(
        name = "io_opencensus_go",
        importpath = "go.opencensus.io",
        sum = "h1:y73uSU6J157QMP2kn2r30vwW1A2W2WFwSCGnAVxeaD0=",
        version = "v0.24.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel",
        importpath = "go.opentelemetry.io/otel",
        sum = "h1:PdomN/Al4q/lN6iBJEN3AwPvUiHPMlt93c8bqTG5Llw=",
        version = "v1.29.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_exporters_jaeger",
        importpath = "go.opentelemetry.io/otel/exporters/jaeger",
        sum = "h1:D7UpUy2Xc2wsi1Ras6V40q806WM07rqoCWzXu7Sqy+4=",
        version = "v1.17.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_metric",
        importpath = "go.opentelemetry.io/otel/metric",
        sum = "h1:vPf/HFWTNkPu1aYeIsc98l4ktOQaL6LeSoeV2g+8YLc=",
        version = "v1.29.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_sdk",
        importpath = "go.opentelemetry.io/otel/sdk",
        sum = "h1:vkqKjk7gwhS8VaWb0POZKmIEDimRCMsopNYnriHyryo=",
        version = "v1.29.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_trace",
        importpath = "go.opentelemetry.io/otel/trace",
        sum = "h1:J/8ZNK4XgR7a21DZUAsbF8pZ5Jcw1VhACmnYt39JTi4=",
        version = "v1.29.0",
    )
    go_repository(
        name = "io_rsc_binaryregexp",
        importpath = "rsc.io/binaryregexp",
        sum = "h1:HfqmD5MEmC0zvwBuF187nq9mdnXjXsSivRiXN7SmRkE=",
        version = "v0.2.0",
    )
    go_repository(
        name = "io_rsc_quote_v3",
        importpath = "rsc.io/quote/v3",
        sum = "h1:9JKUTTIUgS6kzR9mK1YuGKv6Nl+DijDNIc0ghT58FaY=",
        version = "v3.1.0",
    )
    go_repository(
        name = "io_rsc_sampler",
        importpath = "rsc.io/sampler",
        sum = "h1:7uVkIFmeBqHfdjD+gZwtXXI+RODJ2Wc4O7MPEh/QiW4=",
        version = "v1.3.0",
    )
    go_repository(
        name = "io_rsc_tmplfunc",
        importpath = "rsc.io/tmplfunc",
        sum = "h1:53XFQh69AfOa8Tw0Jm7t+GV7KZhOi6jzsCzTtKbMvzU=",
        version = "v0.0.3",
    )
    go_repository(
        name = "org_apache_git_thrift_git",
        importpath = "git.apache.org/thrift.git",
        sum = "h1:OR8VhtwhcAI3U48/rzBsVOuHi0zDPzYI1xASVcdSgR8=",
        version = "v0.0.0-20180902110319-2566ecd5d999",
    )
    go_repository(
        name = "org_go4",
        importpath = "go4.org",
        sum = "h1:+hE86LblG4AyDgwMCLTE6FOlM9+qjHSYS+rKqxUVdsM=",
        version = "v0.0.0-20180809161055-417644f6feb5",
    )
    go_repository(
        name = "org_go4_grpc",
        importpath = "grpc.go4.org",
        sum = "h1:tmXTu+dfa+d9Evp8NpJdgOy6+rt8/x4yG7qPBrtNfLY=",
        version = "v0.0.0-20170609214715-11d0a25b4919",
    )
    go_repository(
        name = "org_golang_google_api",
        importpath = "google.golang.org/api",
        sum = "h1:uWrpz12dpVPn7cojP82mk02XDgTJLDPc2KbVTxrWb4A=",
        version = "v0.40.0",
    )
    go_repository(
        name = "org_golang_google_appengine",
        importpath = "google.golang.org/appengine",
        sum = "h1:FZR1q0exgwxzPzp/aF+VccGrSfxfPpkBqjIIEq3ru6c=",
        version = "v1.6.7",
    )
    go_repository(
        name = "org_golang_google_genproto",
        importpath = "google.golang.org/genproto",
        sum = "h1:KpwkzHKEF7B9Zxg18WzOa7djJ+Ha5DzthMyZYQfEn2A=",
        version = "v0.0.0-20230410155749-daa745c078e1",
    )
    go_repository(
        name = "org_golang_google_genproto_googleapis_api",
        importpath = "google.golang.org/genproto/googleapis/api",
        sum = "h1:7whR9kGa5LUwFtpLm2ArCEejtnxlGeLbAyjFY8sGNFw=",
        version = "v0.0.0-20240528184218-531527333157",
    )
    go_repository(
        name = "org_golang_google_genproto_googleapis_rpc",
        importpath = "google.golang.org/genproto/googleapis/rpc",
        sum = "h1:Zy9XzmMEflZ/MAaA7vNcoebnRAld7FsPW1EeBB7V0m8=",
        version = "v0.0.0-20240528184218-531527333157",
    )
    go_repository(
        name = "org_golang_google_grpc",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/grpc",
        sum = "h1:bs/cUb4lp1G5iImFFd3u5ixQzweKizoZJAwBNLR42lc=",
        version = "v1.65.0",
    )
    go_repository(
        name = "org_golang_google_protobuf",
        importpath = "google.golang.org/protobuf",
        sum = "h1:6xV6lTsCfpGD21XK49h7MhtcApnLqkfYgPcdHftf6hg=",
        version = "v1.34.2",
    )
    go_repository(
        name = "org_golang_x_build",
        importpath = "golang.org/x/build",
        sum = "h1:E2M5QgjZ/Jg+ObCQAudsXxuTsLj7Nl5RV/lZcQZmKSo=",
        version = "v0.0.0-20190111050920-041ab4dc3f9d",
    )
    go_repository(
        name = "org_golang_x_crypto",
        importpath = "golang.org/x/crypto",
        sum = "h1:RrRspgV4mU+YwB4FYnuBoKsUapNIL5cohGAmSH3azsw=",
        version = "v0.26.0",
    )
    go_repository(
        name = "org_golang_x_exp",
        importpath = "golang.org/x/exp",
        sum = "h1:ELnwvuAXPNtPk1TJRuGkI9fDTwym6AYBu0qzT8AcHdI=",
        version = "v0.0.0-20240808152545-0cdaa3abc0fa",
    )
    go_repository(
        name = "org_golang_x_exp_typeparams",
        importpath = "golang.org/x/exp/typeparams",
        sum = "h1:1P7xPZEwZMoBoz0Yze5Nx2/4pxj6nw9ZqHWXqP0iRgQ=",
        version = "v0.0.0-20231108232855-2478ac86f678",
    )
    go_repository(
        name = "org_golang_x_image",
        importpath = "golang.org/x/image",
        sum = "h1:+qEpEAPhDZ1o0x3tHzZTQDArnOixOzGD9HUJfcg0mb4=",
        version = "v0.0.0-20190802002840-cff245a6509b",
    )
    go_repository(
        name = "org_golang_x_lint",
        importpath = "golang.org/x/lint",
        sum = "h1:2M3HP5CCK1Si9FQhwnzYhXdG6DXeebvUHFpre8QvbyI=",
        version = "v0.0.0-20201208152925-83fdc39ff7b5",
    )
    go_repository(
        name = "org_golang_x_mobile",
        importpath = "golang.org/x/mobile",
        sum = "h1:4+4C/Iv2U4fMZBiMCc98MG1In4gJY5YRhtpDNeDeHWs=",
        version = "v0.0.0-20190719004257-d2bd2a29d028",
    )
    go_repository(
        name = "org_golang_x_mod",
        importpath = "golang.org/x/mod",
        sum = "h1:utOm6MM3R3dnawAiJgn0y+xvuYRsm1RKM/4giyfDgV0=",
        version = "v0.20.0",
    )
    go_repository(
        name = "org_golang_x_net",
        importpath = "golang.org/x/net",
        sum = "h1:a9JDOJc5GMUJ0+UDqmLT86WiEy7iWyIhz8gz8E4e5hE=",
        version = "v0.28.0",
    )
    go_repository(
        name = "org_golang_x_oauth2",
        importpath = "golang.org/x/oauth2",
        sum = "h1:tsimM75w1tF/uws5rbeHzIWxEqElMehnc+iW793zsZs=",
        version = "v0.21.0",
    )
    go_repository(
        name = "org_golang_x_perf",
        importpath = "golang.org/x/perf",
        sum = "h1:ObuXPmIgI4ZMyQLIz48cJYgSyWdjUXc2SZAdyJMwEAU=",
        version = "v0.0.0-20230113213139-801c7ef9e5c5",
    )
    go_repository(
        name = "org_golang_x_sync",
        importpath = "golang.org/x/sync",
        sum = "h1:3NFvSEYkUoMifnESzZl15y791HH1qU2xm6eCJU5ZPXQ=",
        version = "v0.8.0",
    )
    go_repository(
        name = "org_golang_x_sys",
        importpath = "golang.org/x/sys",
        sum = "h1:Twjiwq9dn6R1fQcyiK+wQyHWfaz/BJB+YIpzU/Cv3Xg=",
        version = "v0.24.0",
    )
    go_repository(
        name = "org_golang_x_telemetry",
        importpath = "golang.org/x/telemetry",
        sum = "h1:zf5N6UOrA487eEFacMePxjXAJctxKmyjKUsjA11Uzuk=",
        version = "v0.0.0-20240521205824-bda55230c457",
    )
    go_repository(
        name = "org_golang_x_term",
        importpath = "golang.org/x/term",
        sum = "h1:F6D4vR+EHoL9/sWAWgAR1H2DcHr4PareCbAaCo1RpuU=",
        version = "v0.23.0",
    )
    go_repository(
        name = "org_golang_x_text",
        importpath = "golang.org/x/text",
        sum = "h1:XtiM5bkSOt+ewxlOE/aE/AKEHibwj/6gvWMl9Rsh0Qc=",
        version = "v0.17.0",
    )
    go_repository(
        name = "org_golang_x_time",
        importpath = "golang.org/x/time",
        sum = "h1:o7cqy6amK/52YcAKIPlM3a+Fpj35zvRj2TP+e1xFSfk=",
        version = "v0.5.0",
    )
    go_repository(
        name = "org_golang_x_tools",
        importpath = "golang.org/x/tools",
        sum = "h1:J1shsA93PJUEVaUSaay7UXAyE8aimq3GW0pjlolpa24=",
        version = "v0.24.0",
    )
    go_repository(
        name = "org_golang_x_xerrors",
        importpath = "golang.org/x/xerrors",
        sum = "h1:go1bK/D/BFZV2I8cIQd1NKEZ+0owSTG1fDTci4IqFcE=",
        version = "v0.0.0-20200804184101-5ec99f83aff1",
    )
    go_repository(
        name = "org_uber_go_atomic",
        importpath = "go.uber.org/atomic",
        sum = "h1:ADUqmZGgLDDfbSL9ZmPxKTybcoEYHgpYfELNoN+7hsw=",
        version = "v1.7.0",
    )
    go_repository(
        name = "org_uber_go_automaxprocs",
        build_directives = [
            # Do not use this library directly.
            # Rather, load maxprocs from github.com/prysmaticlabs/runtime/maxprocs.
            "gazelle:go_visibility @prysm//runtime/maxprocs:__pkg__",
        ],
        importpath = "go.uber.org/automaxprocs",
        sum = "h1:2LxUOGiR3O6tw8ui5sZa2LAaHnsviZdVOUZw4fvbnME=",
        version = "v1.5.2",
    )
    go_repository(
        name = "org_uber_go_dig",
        importpath = "go.uber.org/dig",
        sum = "h1:imUL1UiY0Mg4bqbFfsRQO5G4CGRBec/ZujWTvSVp3pw=",
        version = "v1.18.0",
    )
    go_repository(
        name = "org_uber_go_fx",
        importpath = "go.uber.org/fx",
        sum = "h1:iPW+OPxv0G8w75OemJ1RAnTUrF55zOJlXlo1TbJ0Buw=",
        version = "v1.22.2",
    )
    go_repository(
        name = "org_uber_go_goleak",
        importpath = "go.uber.org/goleak",
        sum = "h1:2K3zAYmnTNqV73imy9J1T3WC+gmCePx2hEGkimedGto=",
        version = "v1.3.0",
    )
    go_repository(
        name = "org_uber_go_mock",
        importpath = "go.uber.org/mock",
        sum = "h1:VcM4ZOtdbR4f6VXfiOpwpVJDL6lCReaZ6mw31wqh7KU=",
        version = "v0.4.0",
    )
    go_repository(
        name = "org_uber_go_multierr",
        importpath = "go.uber.org/multierr",
        sum = "h1:blXXJkSxSSfBVBlC76pxqeO+LN3aDfLQo+309xJstO0=",
        version = "v1.11.0",
    )
    go_repository(
        name = "org_uber_go_tools",
        importpath = "go.uber.org/tools",
        sum = "h1:0mgffUl7nfd+FpvXMVz4IDEaUSmT1ysygQC7qYo7sG4=",
        version = "v0.0.0-20190618225709-2cfd321de3ee",
    )
    go_repository(
        name = "org_uber_go_zap",
        importpath = "go.uber.org/zap",
        sum = "h1:aJMhYGrd5QSmlpLMr2MftRKl7t8J8PTZPA732ud/XR8=",
        version = "v1.27.0",
    )

    http_archive(
        name = "com_github_supranational_blst",
        urls = [
            "https://github.com/supranational/blst/archive/3dd0f804b1819e5d03fb22ca2e6fac105932043a.tar.gz",
        ],
        strip_prefix = "blst-3dd0f804b1819e5d03fb22ca2e6fac105932043a",
        build_file = "//third_party:blst/blst.BUILD",
        sha256 = "132124c074e59ead77e1828cc54b587a182ea67b781b72198e802af4696d78fe",
    )
