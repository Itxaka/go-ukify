package pcr

import (
	"github.com/google/go-tpm/tpm2"
	"github.com/kairos-io/go-ukify/pkg/constants"
	"github.com/kairos-io/go-ukify/pkg/pesign"
	"github.com/kairos-io/go-ukify/pkg/types"
	"github.com/siderolabs/gen/xslices"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PCR test Suite")
}

// SectionsData transforms a []types.UkiSection into a map[constants.Section]string
// based on types.UkiSection.Measure being true
// So it obtains a list of sections that have to be measured
// TODO: use it everywhere instead of just under tests?
func SectionsData(sections []types.UkiSection) map[constants.Section]string {
	return xslices.ToMap(
		xslices.Filter(sections,
			func(s types.UkiSection) bool {
				return s.Measure
			},
		),
		func(s types.UkiSection) (constants.Section, string) {
			return s.Name, s.Path
		})
}

// Hashes precalculated manually and with other tools
// For the different ordered phases (enter-initrd,leave-initrd,sysinit and ready) for empty
// section data
// Ideally once we allow passing the phases to the calculatebankdata function we can modify this to generate only 1
// policy for easy testing
var knowPCR11PolicyHashFirstPhase = "7c8486f61cc1d88a28d6ab87850bee07c467ce6311340219e43a7a6e6521e543"
var knowPCR11PolicyHashSecondPhase = "7474e6080ddc5355c6087db4272c7d8a6871a7c83a54694369561253f08fd3f1"
var knowPCR11PolicyHashThirdPhase = "8fac790c125cc6c82b372714c8ecf83784523c05c5b78b37b1aae05521b7ec3e"
var knowPCR11PolicyHashFourthPhase = "53f5e6ee03093e2fb1ea9d1351952a33ce381ae93bef210abb764941be8d8ec6"

var _ = Describe("PCR tests", func() {
	var cmdlineSection types.UkiSection
	var unameSection types.UkiSection
	var tmpDir string
	var pcrsigner *pesign.PCRSigner
	var err error

	BeforeEach(func() {
		pcrsigner, err = pesign.NewPCRSigner("testdata/private.pem")
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err := os.MkdirTemp("", "")
		Expect(err).ToNot(HaveOccurred())

		err = os.WriteFile(filepath.Join(tmpDir, "cmdline"), []byte("root=LABEL=BOOT"), 777)
		Expect(err).ToNot(HaveOccurred())

		err = os.WriteFile(filepath.Join(tmpDir, "uname"), []byte("6.5.0"), 777)
		Expect(err).ToNot(HaveOccurred())

		cmdlineSection = types.UkiSection{
			Name:    constants.CMDLine,
			Path:    filepath.Join(tmpDir, "cmdline"),
			Measure: true,
		}

		unameSection = types.UkiSection{
			Name:    constants.Uname,
			Path:    filepath.Join(tmpDir, "cmdline"),
			Measure: true,
		}

	})
	AfterEach(func() {
		err := os.RemoveAll(tmpDir)
		Expect(err).ToNot(HaveOccurred())
	})
	Describe("Bank", func() {
		Describe("CalculateBankData", func() {
			It("Calculates the policy hash for empty sections", Focus, func() {
				sectionsData := SectionsData([]types.UkiSection{})
				//data, err := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				Expect(err).ToNot(HaveOccurred())
				var data *types.PCRData
				var algos []types.Algorithm
				data, algos = types.GetTPMALGorithm()
				for _, alg := range algos {
					banks := make([]types.BankData, 0)
					hash, err := MeasureSections(alg.Alg, sectionsData)
					Expect(err).ToNot(HaveOccurred())
					for _, phase := range types.OrderedPhases() {
						hash = MeasurePhase(phase, alg.Alg, hash)
						bank, err := SignPolicy(11, alg.Alg, pcrsigner, hash)
						Expect(err).ToNot(HaveOccurred())
						banks = append(banks, bank)
					}
					*alg.BankDataSetter = banks
				}
				// old method
				data2sha1, _ := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA1, sectionsData, pcrsigner)
				data2sha256, _ := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				data2sha384, _ := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA384, sectionsData, pcrsigner)
				data2sha512, _ := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA512, sectionsData, pcrsigner)
				Expect(len(data.SHA256)).ToNot(Equal(0))
				Expect(data.SHA256[0].Pol).To(Equal(knowPCR11PolicyHashFirstPhase))
				Expect(data.SHA256[1].Pol).To(Equal(knowPCR11PolicyHashSecondPhase))
				Expect(data.SHA256[2].Pol).To(Equal(knowPCR11PolicyHashThirdPhase))
				Expect(data.SHA256[3].Pol).To(Equal(knowPCR11PolicyHashFourthPhase))
				// Check that new methods return the same data as before
				Expect(data.SHA1).To(Equal(data2sha1))
				Expect(data.SHA256).To(Equal(data2sha256))
				Expect(data.SHA384).To(Equal(data2sha384))
				Expect(data.SHA512).To(Equal(data2sha512))
			})
			It("Does not calculate the same policy hash for a different PCR", func() {
				sectionsData := SectionsData([]types.UkiSection{})
				// Using PCR13 instead of PCR11
				var data *types.PCRData
				alg := tpm2.TPMAlgSHA256
				banks := make([]types.BankData, 0)
				hash, err := MeasureSections(alg.Alg, sectionsData)
				Expect(err).ToNot(HaveOccurred())
				for _, phase := range types.OrderedPhases() {
					hash = MeasurePhase(phase, alg.Alg, hash)
					bank, err := SignPolicy(11, alg.Alg, pcrsigner, hash)
					Expect(err).ToNot(HaveOccurred())
					banks = append(banks, bank)
				}
				*alg.BankDataSetter = banks

				data, err := CalculateBankData(13, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(data)).ToNot(Equal(0))
				Expect(data[0].Pol).ToNot(Equal(knowPCR11PolicyHashFirstPhase))
				Expect(data[1].Pol).ToNot(Equal(knowPCR11PolicyHashSecondPhase))
				Expect(data[2].Pol).ToNot(Equal(knowPCR11PolicyHashThirdPhase))
				Expect(data[3].Pol).ToNot(Equal(knowPCR11PolicyHashFourthPhase))
			})
			It("Policy hash doesnt match when changing the sections", func() {
				sectionsData := SectionsData([]types.UkiSection{})

				data, err := CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(data)).ToNot(Equal(0))
				Expect(data[0].Pol).To(Equal(knowPCR11PolicyHashFirstPhase))
				Expect(data[1].Pol).To(Equal(knowPCR11PolicyHashSecondPhase))
				Expect(data[2].Pol).To(Equal(knowPCR11PolicyHashThirdPhase))
				Expect(data[3].Pol).To(Equal(knowPCR11PolicyHashFourthPhase))

				sectionsData = SectionsData([]types.UkiSection{
					cmdlineSection,
				})
				data, err = CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(data)).ToNot(Equal(0))
				Expect(data[0].Pol).ToNot(Equal(knowPCR11PolicyHashFirstPhase))
				Expect(data[1].Pol).ToNot(Equal(knowPCR11PolicyHashSecondPhase))
				Expect(data[2].Pol).ToNot(Equal(knowPCR11PolicyHashThirdPhase))
				Expect(data[3].Pol).ToNot(Equal(knowPCR11PolicyHashFourthPhase))

				sectionsData = SectionsData([]types.UkiSection{
					cmdlineSection,
					unameSection,
				})
				data, err = CalculateBankData(11, types.OrderedPhases(), tpm2.TPMAlgSHA256, sectionsData, pcrsigner)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(data)).ToNot(Equal(0))
				Expect(data[0].Pol).ToNot(Equal(knowPCR11PolicyHashFirstPhase))
				Expect(data[1].Pol).ToNot(Equal(knowPCR11PolicyHashSecondPhase))
				Expect(data[2].Pol).ToNot(Equal(knowPCR11PolicyHashThirdPhase))
				Expect(data[3].Pol).ToNot(Equal(knowPCR11PolicyHashFourthPhase))

			})

		})
		Describe("CreateSelector", func() {
			It("Returns expected mask", func() {
				selector, err := CreateSelector([]int{0})
				Expect(err).ToNot(HaveOccurred())
				Expect(selector).To(Equal([]uint8{1, 0, 0}))
				selector, err = CreateSelector([]int{1})
				Expect(err).ToNot(HaveOccurred())
				Expect(selector).To(Equal([]uint8{2, 0, 0}))
				selector, err = CreateSelector([]int{1, 2})
				Expect(err).ToNot(HaveOccurred())
				Expect(selector).To(Equal([]uint8{6, 0, 0}))
				selector, err = CreateSelector([]int{3})
				Expect(err).ToNot(HaveOccurred())
				Expect(selector).To(Equal([]uint8{8, 0, 0}))
			})
			It("Returns an error if we go over the PCR index range(24)", func() {
				_, err := CreateSelector([]int{24})
				Expect(err).To(HaveOccurred())
			})
		})
		Describe("CalculatePolicy", func() {
			It("Generates the proper signed policy", func() {
				pcrSelector, err := CreateSelector([]int{11})
				Expect(err).ToNot(HaveOccurred())

				pcrSelection := tpm2.TPMLPCRSelection{
					PCRSelections: []tpm2.TPMSPCRSelection{
						{
							Hash:      tpm2.TPMAlgSHA256,
							PCRSelect: pcrSelector,
						},
					},
				}

				hashAlg, err := tpm2.TPMAlgSHA256.Hash()
				hashData := NewDigest(hashAlg)
				hashData.Extend([]byte("enter-initrd"))
				hash := hashData.Hash()

				policyPCR, err := CalculatePolicy(hash, pcrSelection)
				sigData, err := Sign(policyPCR, hashAlg, pcrsigner)
				// This should match the same data that we got from the CalculateBankData with empty sections.
				// I.e. This hashes empty data and then hashes the "enter-initrd" string, so it should match the same
				// data as we pre-calculated
				Expect(sigData.Digest).To(Equal(knowPCR11PolicyHashFirstPhase))

				// Now we hash with the second "leave-initrd" phase and calculate the policy again
				hashData.Extend([]byte("leave-initrd"))
				hash = hashData.Hash()

				policyPCR, err = CalculatePolicy(hash, pcrSelection)
				sigData, err = Sign(policyPCR, hashAlg, pcrsigner)
				Expect(sigData.Digest).To(Equal(knowPCR11PolicyHashSecondPhase))

				// And again with sysinit
				hashData.Extend([]byte("sysinit"))
				hash = hashData.Hash()

				policyPCR, err = CalculatePolicy(hash, pcrSelection)
				sigData, err = Sign(policyPCR, hashAlg, pcrsigner)
				Expect(sigData.Digest).To(Equal(knowPCR11PolicyHashThirdPhase))

				// And finally with ready
				hashData.Extend([]byte("ready"))
				hash = hashData.Hash()

				policyPCR, err = CalculatePolicy(hash, pcrSelection)
				sigData, err = Sign(policyPCR, hashAlg, pcrsigner)
				Expect(sigData.Digest).To(Equal(knowPCR11PolicyHashFourthPhase))

			})
		})

	})
	Describe("Extend", func() {
		It("Extends the hash properly", func() {
			hashAlg, err := tpm2.TPMAlgSHA256.Hash()
			Expect(err).ToNot(HaveOccurred())
			hash := NewDigest(hashAlg)
			// Expect it to be empty
			Expect(hash.Hash()).To(Equal(make([]byte, hashAlg.Size())))
			hash.Extend([]byte("Hello"))
			// Expect it to have changed
			Expect(hash.Hash()).ToNot(Equal(make([]byte, hashAlg.Size())))
			// Check against precalculated values
			Expect(hash.Hash()).ToNot(Equal([]byte("5d34a81817bcb7f1856a6e0484572077846d73e9ac5c82bac8d1ee049e2db43e")))
		})
	})
})
